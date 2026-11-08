package resend_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/resend"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

const secret = "whsec_" + "c2VjcmV0LXNpZ25pbmcta2V5LWZvci10ZXN0cw=="

type request struct {
	method      string
	path        string
	idempotency string
	auth        string
	body        map[string]any
}

type fake struct {
	*httptest.Server

	mu       sync.Mutex
	requests []request
	answer   map[string]int
}

func newFake(t *testing.T, answer map[string]int) *fake {
	t.Helper()

	f := &fake{answer: answer}

	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)

		var body map[string]any

		_ = json.Unmarshal(raw, &body)

		f.mu.Lock()
		f.requests = append(f.requests, request{
			method: r.Method, path: r.URL.Path, idempotency: r.Header.Get("Idempotency-Key"),
			auth: r.Header.Get("Authorization"), body: body,
		})
		status, listed := f.answer[r.Method+" "+r.URL.Path]
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if listed && status >= http.StatusBadRequest {
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"statusCode":%d,"name":"failure","message":"failed"}`, status)

			return
		}

		_, _ = w.Write([]byte(`{"id":"re_123","object":"email"}`))
	}))
	t.Cleanup(f.Close)

	return f
}

func (f *fake) seen() []request {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]request(nil), f.requests...)
}

func build(t *testing.T, c resend.Config) *resend.Resend {
	t.Helper()

	r, err := resend.New(c)
	require.NoError(t, err)

	return r
}

func client(t *testing.T, f *fake, segment string) *resend.Resend {
	t.Helper()

	return build(t, resend.Config{
		APIKey: "re_key", BaseURL: f.URL + "/", From: "Desk <desk@example.com>", SegmentID: segment, WebhookSecret: secret,
	})
}

func Test_New_RejectsABaseURLItCannotUse(t *testing.T) {
	_, err := resend.New(resend.Config{BaseURL: "://nope"})

	require.Error(t, err)
}

func email() domain.Email {
	return domain.Email{
		ID: uuid.New(), Recipient: "a@b.co", Subject: "subject", HTML: "<p>x</p>", Text: "x",
	}
}

func Test_Send_DeliversTheLetterWithAnIdempotencyKey(t *testing.T) {
	f := newFake(t, nil)
	e := email()

	id, err := client(t, f, "").Send(t.Context(), e)

	require.NoError(t, err)
	require.Equal(t, "re_123", id)

	got := f.seen()[0]
	require.Equal(t, "POST", got.method)
	require.Equal(t, "/emails", got.path)
	require.Equal(t, e.ID.String(), got.idempotency)
	require.Equal(t, "Bearer re_key", got.auth)
	require.Equal(t, "Desk <desk@example.com>", got.body["from"])
	require.Equal(t, []any{"a@b.co"}, got.body["to"])
	require.Equal(t, "subject", got.body["subject"])
	require.Equal(t, "<p>x</p>", got.body["html"])
	require.Equal(t, "x", got.body["text"])
}

func Test_Send_ARejectedLetterIsUndeliverable(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
		f := newFake(t, map[string]int{"POST /emails": status})

		_, err := client(t, f, "").Send(t.Context(), email())

		require.ErrorIs(t, err, domain.ErrUndeliverable, "status %d", status)
	}
}

func Test_Send_TemporaryFailuresAreRetryable(t *testing.T) {
	statuses := []int{
		http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable,
		http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict,
	}

	for _, status := range statuses {
		f := newFake(t, map[string]int{"POST /emails": status})

		_, err := client(t, f, "").Send(t.Context(), email())

		require.Error(t, err, "status %d", status)
		require.NotErrorIs(t, err, domain.ErrUndeliverable, "status %d", status)
	}
}

func Test_Subscribe_UpdatesAnExistingContactAndAddsItToTheSegment(t *testing.T) {
	f := newFake(t, nil)

	require.NoError(t, client(t, f, "seg-1").Subscribe(t.Context(), "a@b.co"))

	got := f.seen()
	require.Len(t, got, 2)
	require.Equal(t, "PATCH", got[0].method)
	require.Equal(t, "/contacts/a@b.co", got[0].path)
	require.Equal(t, false, got[0].body["unsubscribed"])
	require.Equal(t, "POST", got[1].method)
	require.Equal(t, "/contacts/a@b.co/segments/seg-1", got[1].path)
}

func Test_Subscribe_CreatesAContactThatDoesNotExistYet(t *testing.T) {
	f := newFake(t, map[string]int{"PATCH /contacts/a@b.co": http.StatusNotFound})

	require.NoError(t, client(t, f, "seg-1").Subscribe(t.Context(), "a@b.co"))

	got := f.seen()
	require.Len(t, got, 2)
	require.Equal(t, "POST", got[1].method)
	require.Equal(t, "/contacts", got[1].path)
	require.Equal(t, "a@b.co", got[1].body["email"])
	require.Equal(t, []any{map[string]any{"id": "seg-1"}}, got[1].body["segments"])
}

func Test_Subscribe_WithoutASegmentOnlyUpdatesTheContact(t *testing.T) {
	f := newFake(t, nil)

	require.NoError(t, client(t, f, "").Subscribe(t.Context(), "a@b.co"))
	require.Len(t, f.seen(), 1)
}

func Test_Subscribe_AContactAlreadyInTheSegmentIsFine(t *testing.T) {
	f := newFake(t, map[string]int{"POST /contacts/a@b.co/segments/seg-1": http.StatusConflict})

	require.NoError(t, client(t, f, "seg-1").Subscribe(t.Context(), "a@b.co"))
}

func Test_Subscribe_ReportsAFailure(t *testing.T) {
	f := newFake(t, map[string]int{"PATCH /contacts/a@b.co": http.StatusInternalServerError})

	require.Error(t, client(t, f, "").Subscribe(t.Context(), "a@b.co"))
}

func Test_Unsubscribe_MarksTheContact(t *testing.T) {
	f := newFake(t, nil)

	require.NoError(t, client(t, f, "seg-1").Unsubscribe(t.Context(), "a@b.co"))

	got := f.seen()
	require.Len(t, got, 1)
	require.Equal(t, "PATCH", got[0].method)
	require.Equal(t, true, got[0].body["unsubscribed"])
}

func Test_Unsubscribe_AContactThatDoesNotExistIsFine(t *testing.T) {
	f := newFake(t, map[string]int{"PATCH /contacts/a@b.co": http.StatusNotFound})

	require.NoError(t, client(t, f, "").Unsubscribe(t.Context(), "a@b.co"))
}

func sign(id, timestamp, payload string) string {
	key, _ := base64.StdEncoding.DecodeString(secret[len("whsec_"):])
	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "%s.%s.%s", id, timestamp, payload)

	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func now() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

func Test_ParseWebhook_ReadsAnUnsubscribe(t *testing.T) {
	f := newFake(t, nil)
	payload := `{"type":"contact.updated","data":{"email":"a@b.co","unsubscribed":true}}`
	stamp := now()

	event, err := client(t, f, "").ParseWebhook([]byte(payload), "msg_1", stamp, sign("msg_1", stamp, payload))

	require.NoError(t, err)
	require.Equal(t, domain.ContactEvent{Email: "a@b.co", Unsubscribed: true}, event)
}

func Test_ParseWebhook_IgnoresOtherEvents(t *testing.T) {
	f := newFake(t, nil)
	payload := `{"type":"email.delivered","data":{"email_id":"x"}}`
	stamp := now()

	event, err := client(t, f, "").ParseWebhook([]byte(payload), "msg_1", stamp, sign("msg_1", stamp, payload))

	require.NoError(t, err)
	require.Equal(t, domain.ContactEvent{}, event)
}

func Test_ParseWebhook_RejectsABadSignature(t *testing.T) {
	f := newFake(t, nil)
	payload := `{"type":"contact.updated","data":{"email":"a@b.co","unsubscribed":true}}`
	stamp := now()

	_, err := client(t, f, "").ParseWebhook([]byte(payload), "msg_1", stamp, sign("msg_2", stamp, payload))

	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func Test_ParseWebhook_RejectsAnOldDelivery(t *testing.T) {
	f := newFake(t, nil)
	payload := `{"type":"contact.updated","data":{"email":"a@b.co","unsubscribed":true}}`
	stamp := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)

	_, err := client(t, f, "").ParseWebhook([]byte(payload), "msg_1", stamp, sign("msg_1", stamp, payload))

	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func Test_ParseWebhook_FailsClosedWithoutASecret(t *testing.T) {
	payload := `{"type":"contact.updated","data":{"email":"a@b.co","unsubscribed":true}}`
	stamp := now()

	_, err := build(t, resend.Config{APIKey: "k"}).ParseWebhook([]byte(payload), "m", stamp, sign("m", stamp, payload))

	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func Test_Failures_NeverCarryTheAddress(t *testing.T) {
	f := newFake(t, nil)
	r := client(t, f, "")

	f.Close()

	err := r.Subscribe(t.Context(), "alice@b.co")

	require.Error(t, err)
	require.NotContains(t, err.Error(), "alice@b.co")
}

func Test_Traces_NeverCarryTheAddress(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	previous := otel.GetTracerProvider()

	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	f := newFake(t, nil)
	r := client(t, f, "seg-1")

	require.NoError(t, r.Subscribe(t.Context(), "alice@b.co"))
	require.NoError(t, r.Unsubscribe(t.Context(), "alice@b.co"))

	_, err := r.Send(t.Context(), email())
	require.NoError(t, err)

	spans := recorder.Ended()
	require.NotEmpty(t, spans, "sending an email is still traced")

	for _, span := range spans {
		require.NotContains(t, span.Name(), "alice@b.co")

		for _, attribute := range span.Attributes() {
			require.NotContains(t, attribute.Value.Emit(), "alice@b.co", string(attribute.Key))
		}
	}
}
