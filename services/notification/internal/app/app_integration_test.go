//go:build integration

package app_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/kafka"
	"github.com/vbloher/canton-private-otc-desk/pkg/migrate"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/outbox"
	"github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/pkg/tradeevents"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
	"github.com/vbloher/canton-private-otc-desk/services/notification/config"
	notificationredis "github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/redis"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/resend"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/app"
	kafkacontroller "github.com/vbloher/canton-private-otc-desk/services/notification/internal/controller/kafka"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/controller/worker"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
	tradingmigration "github.com/vbloher/canton-private-otc-desk/services/trading/migration"
)

const (
	webhookSecret = "whsec_" + "c2VjcmV0LXNpZ25pbmcta2V5LWZvci10ZXN0cw=="
	eventually    = 30 * time.Second
	tick          = 25 * time.Millisecond
)

var errBrokerDown = errors.New("the broker is down")

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func unique(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
}

func databaseConfig(name string) postgres.Config {
	return postgres.Config{
		User:     getenv("POSTGRES_USER", "otc"),
		Password: getenv("POSTGRES_PASSWORD", "otc"),
		Port:     getenv("POSTGRES_PORT", "5432"),
		Host:     getenv("POSTGRES_HOST", "localhost"),
		DBName:   name,
	}
}

func dropAfter(t *testing.T, name string) {
	t.Helper()

	t.Cleanup(func() {
		admin, err := postgres.New(context.Background(), databaseConfig("postgres"))
		require.NoError(t, err)

		defer admin.Close()

		_, err = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		require.NoError(t, err)
	})
}

type letter struct {
	To          string   `json:"-"`
	Recipients  []string `json:"to"`
	Subject     string   `json:"subject"`
	HTML        string   `json:"html"`
	Text        string   `json:"text"`
	Idempotency string   `json:"-"`
}

type fakeResend struct {
	*httptest.Server

	mu       sync.Mutex
	letters  []letter
	contacts []string
}

func startResend(t *testing.T) *fakeResend {
	t.Helper()

	f := &fakeResend{}

	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		f.mu.Lock()
		defer f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPost && r.URL.Path == "/emails" {
			var l letter

			_ = json.Unmarshal(body, &l)
			l.To = strings.Join(l.Recipients, ",")
			l.Idempotency = r.Header.Get("Idempotency-Key")
			f.letters = append(f.letters, l)
		} else {
			f.contacts = append(f.contacts, r.Method+" "+r.URL.Path)
		}

		_, _ = w.Write([]byte(`{"id":"re_1","object":"email"}`))
	}))
	t.Cleanup(f.Close)

	return f
}

func (f *fakeResend) sent(subject string) []letter {
	f.mu.Lock()
	defer f.mu.Unlock()

	var found []letter

	for _, l := range f.letters {
		if l.Subject == subject {
			found = append(found, l)
		}
	}

	return found
}

func (f *fakeResend) contactCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.contacts...)
}

func freePort(t *testing.T) string {
	t.Helper()

	var lc net.ListenConfig

	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	return port
}

type service struct {
	baseURL string
}

func (s service) call(t *testing.T, method, path, body, party string, headers map[string]string) (int, string) {
	t.Helper()

	r, err := http.NewRequestWithContext(t.Context(), method, s.baseURL+path, strings.NewReader(body))
	require.NoError(t, err)

	r.Header.Set("Content-Type", "application/json")

	if party != "" {
		principal.Principal{UserID: uuid.New(), PartyID: party, Hint: "hint"}.SetHeaders(r.Header)
	}

	for key, value := range headers {
		r.Header.Set(key, value)
	}

	response, err := http.DefaultClient.Do(r)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, string(raw)
}

func kafkaConfig() kafka.Config {
	return kafka.Config{Brokers: strings.Split(getenv("KAFKA_BROKERS", "localhost:9094"), ",")}
}

func start(t *testing.T, fake *fakeResend) service {
	t.Helper()

	otel.SilentModeInit()

	name := unique("notification_e2e_")
	dropAfter(t, name)

	port := freePort(t)

	c := config.Config{
		HTTP:     httpserver.Config{Host: "127.0.0.1", Port: port},
		Postgres: databaseConfig(name),
		Redis:    redis.Config{Addr: getenv("REDIS_ADDR", "localhost:6379")},
		Kafka:    kafkaConfig(),
		Consumer: kafkacontroller.Config{Group: unique("e2e-")},
		Limits:   notificationredis.Config{DailyLimit: 1 << 30},
		Email: resend.Config{
			APIKey: "re_key", BaseURL: fake.URL + "/", From: "Desk <desk@example.com>",
			SegmentID: "seg-1", WebhookSecret: webhookSecret,
		},
		UseCase: usecase.Config{PublicURL: "https://desk.example.com", ConfirmationTTL: time.Hour, Enabled: true},
		Worker:  worker.Config{Interval: tick},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- app.Run(ctx, c) }()

	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})

	s := service{baseURL: "http://127.0.0.1:" + port}

	require.Eventually(t, func() bool {
		r, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.baseURL+"/ready", nil)
		if err != nil {
			return false
		}

		response, err := http.DefaultClient.Do(r)
		if err != nil {
			return false
		}

		defer func() { _ = response.Body.Close() }()

		return response.StatusCode == http.StatusOK
	}, eventually, tick, "the service did not start")

	return s
}

type tradingSide struct {
	t  *testing.T
	tx *transaction.Manager
}

func newTrading(t *testing.T) tradingSide {
	t.Helper()

	name := unique("trading_e2e_")
	c := databaseConfig(name)

	require.NoError(t, postgres.EnsureDatabase(t.Context(), c))
	dropAfter(t, name)
	require.NoError(t, migrate.Up(c.URL(), tradingmigration.FS))

	pool, err := postgres.New(t.Context(), c)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return tradingSide{t: t, tx: transaction.New(pool)}
}

func (s tradingSide) record(m outbox.Message) {
	s.t.Helper()

	require.NoError(s.t, s.tx.Wrap(s.t.Context(), func(ctx context.Context) error {
		return outbox.NewWriter(s.tx).Add(ctx, m)
	}))
}

func (s tradingSide) relay(p outbox.Producer) *outbox.Relay {
	s.t.Helper()

	r, err := outbox.NewRelay(outbox.Config{Interval: time.Hour, BatchSize: 10, Retention: time.Hour}, s.tx, p)
	require.NoError(s.t, err)
	s.t.Cleanup(r.Close)

	return r
}

type downBroker struct{}

func (downBroker) Produce(context.Context, ...kafka.Record) error {
	return errBrokerDown
}

func settledEvent(taker, maker string) tradeevents.Settled {
	return tradeevents.Settled{
		TradeID:   uuid.New(),
		UpdateID:  "1220deadbeef",
		SettledAt: time.Now(),
		Taker: tradeevents.Side{
			PartyID:  taker,
			Gave:     tradeevents.Amount{Symbol: "CC", Value: decimal.MustParse("12345")},
			Received: tradeevents.Amount{Symbol: "CBTC", Value: decimal.MustParse("0.5")},
		},
		Maker: tradeevents.Side{
			PartyID:  maker,
			Gave:     tradeevents.Amount{Symbol: "CBTC", Value: decimal.MustParse("0.5")},
			Received: tradeevents.Amount{Symbol: "CC", Value: decimal.MustParse("12332.655")},
		},
	}
}

func messageOf(t *testing.T, event tradeevents.Settled) outbox.Message {
	t.Helper()

	m, err := outbox.NewMessage(tradeevents.Topic, event.TradeID.String(), tradeevents.TypeSettled, event)
	require.NoError(t, err)

	return m
}

func confirmAddress(t *testing.T, s service, fake *fakeResend, party, email string) {
	t.Helper()

	status, body := s.call(t, http.MethodPut, "/api/v1/profile/email", `{"email":"`+email+`"}`, party, nil)
	require.Equal(t, http.StatusOK, status, body)
	require.JSONEq(t, `{"email":"`+email+`","emailStatus":"unconfirmed","news":false,"trades":false,"emailEnabled":true}`, body)

	require.Eventually(t, func() bool { return len(fake.sent("Confirm your email for Private OTC Desk")) == 1 }, eventually, tick)

	sent := fake.sent("Confirm your email for Private OTC Desk")[0]
	require.Equal(t, email, sent.To)

	match := regexp.MustCompile(`/confirm-email#token=([\w-]+)`).FindStringSubmatch(sent.Text)
	require.NotNil(t, match, sent.Text)

	confirm := `{"token":"` + match[1] + `"}`

	for range 2 {
		status, body = s.call(t, http.MethodPost, "/api/v1/email-confirmations", confirm, "", nil)
		require.Equal(t, http.StatusNoContent, status, "opening the link twice is still a success: %s", body)
	}

	status, body = s.call(t, http.MethodGet, "/api/v1/profile", "", party, nil)
	require.Equal(t, http.StatusOK, status, body)
	require.JSONEq(t, `{"email":"`+email+`","emailStatus":"confirmed","news":false,"trades":false,"emailEnabled":true}`, body)
}

func signedHeaders(payload string) map[string]string {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	key, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(webhookSecret, "whsec_"))
	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "msg_1.%s.%s", timestamp, payload)

	return map[string]string{
		"Svix-Id":        "msg_1",
		"Svix-Timestamp": timestamp,
		"Svix-Signature": "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)),
	}
}

func Test_Notification(t *testing.T) {
	fake := startResend(t)
	s := start(t, fake)

	t.Run("a trade settled in the outbox becomes one email", func(t *testing.T) {
		aTradeBecomesOneEmail(t, s, fake)
	})

	t.Run("an event for people who did not ask is not an email", func(t *testing.T) {
		anEventForNobodyIsNotAnEmail(t, s, fake)
	})
}

func aTradeBecomesOneEmail(t *testing.T, s service, fake *fakeResend) {
	t.Helper()

	trading := newTrading(t)

	taker := "alice-" + uuid.NewString() + "::1220ab"
	maker := "bob-" + uuid.NewString() + "::1220cd"
	email := strings.ToLower(uuid.NewString()) + "@example.com"

	confirmAddress(t, s, fake, taker, email)

	status, body := s.call(t, http.MethodPut, "/api/v1/profile/notifications", `{"news":true,"trades":true}`, taker, nil)
	require.Equal(t, http.StatusOK, status, body)

	require.Eventually(t, func() bool {
		return strings.Join(fake.contactCalls(), "|") == "PATCH /contacts/"+email+"|POST /contacts/"+email+"/segments/seg-1"
	}, eventually, tick, "the address reaches the news segment")

	event := settledEvent(taker, maker)
	message := messageOf(t, event)

	trading.record(message)

	require.ErrorIs(t, trading.relay(downBroker{}).Tick(t.Context(), time.Now()), errBrokerDown,
		"the event waits in the outbox while the broker is down")

	producer, err := kafka.NewProducer(kafkaConfig())
	require.NoError(t, err)
	t.Cleanup(producer.Close)

	ctx, cancel := context.WithTimeout(t.Context(), eventually)
	defer cancel()

	require.NoError(t, trading.relay(producer).Tick(ctx, time.Now()))

	require.Eventually(t, func() bool { return len(fake.sent("Your trade settled")) == 1 }, eventually, tick)

	sent := fake.sent("Your trade settled")[0]
	require.Equal(t, email, sent.To)
	require.Contains(t, sent.Text, "You gave 12,345 CC and received 0.5 CBTC.")
	require.NotEmpty(t, sent.Idempotency)

	for _, secret := range []string{taker, maker, "1220deadbeef"} {
		require.False(t, bytes.Contains([]byte(sent.HTML+sent.Text+sent.Subject), []byte(secret)), secret)
	}

	duplicate := kafka.Record{
		Topic: tradeevents.Topic, Key: message.Key, Value: message.Payload,
		Headers: map[string]string{outbox.HeaderEventType: tradeevents.TypeSettled},
	}

	sentinel := settledEvent(taker, maker)
	marker := messageOf(t, sentinel)
	after := kafka.Record{
		Topic: tradeevents.Topic, Key: message.Key, Value: marker.Payload,
		Headers: map[string]string{outbox.HeaderEventType: tradeevents.TypeSettled},
	}

	require.NoError(t, producer.Produce(ctx, duplicate, after))

	require.Eventually(t, func() bool { return len(fake.sent("Your trade settled")) >= 2 }, eventually, tick)
	require.Never(t, func() bool { return len(fake.sent("Your trade settled")) > 2 }, time.Second, tick,
		"the redelivered event produced a second letter")

	payload := `{"type":"contact.updated","data":{"email":"` + email + `","unsubscribed":true}}`

	status, body = s.call(t, http.MethodPost, "/api/v1/webhooks/resend", payload, "", signedHeaders(payload))
	require.Equal(t, http.StatusNoContent, status, body)

	status, body = s.call(t, http.MethodGet, "/api/v1/profile", "", taker, nil)
	require.Equal(t, http.StatusOK, status, body)
	require.JSONEq(t, `{"email":"`+email+`","emailStatus":"confirmed","news":false,"trades":true,"emailEnabled":true}`, body)
}

func anEventForNobodyIsNotAnEmail(t *testing.T, s service, fake *fakeResend) {
	t.Helper()

	before := len(fake.sent("Your trade settled"))

	status, body := s.call(t, http.MethodGet, "/api/v1/profile", "", "nobody::1220", nil)
	require.Equal(t, http.StatusOK, status, body)

	producer, err := kafka.NewProducer(kafkaConfig())
	require.NoError(t, err)
	t.Cleanup(producer.Close)

	ctx, cancel := context.WithTimeout(t.Context(), eventually)
	defer cancel()

	message := messageOf(t, settledEvent("nobody-"+uuid.NewString(), "noone-"+uuid.NewString()))

	require.NoError(t, producer.Produce(ctx, kafka.Record{
		Topic: tradeevents.Topic, Key: message.Key, Value: message.Payload,
		Headers: map[string]string{outbox.HeaderEventType: tradeevents.TypeSettled},
	}))

	require.Never(t, func() bool { return len(fake.sent("Your trade settled")) > before }, 2*time.Second, tick)
}
