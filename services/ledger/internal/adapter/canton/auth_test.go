package canton

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func jwt(t *testing.T, claims map[string]any) string {
	t.Helper()

	payload, err := json.Marshal(claims)
	require.NoError(t, err)

	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

type idp struct {
	mu     sync.Mutex
	srv    *httptest.Server
	forms  []url.Values
	token  string
	status int
}

func newIDP(t *testing.T, token string) *idp {
	t.Helper()

	p := &idp{token: token, status: http.StatusOK}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		p.mu.Lock()
		p.forms = append(p.forms, r.PostForm)
		status := p.status
		p.mu.Unlock()

		if status != http.StatusOK {
			http.Error(w, `{"error":"invalid_client"}`, status)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+p.token+`","token_type":"Bearer","expires_in":300}`)
	}))
	t.Cleanup(p.srv.Close)

	return p
}

func (p *idp) auth() AuthConfig {
	return AuthConfig{
		TokenURL:     p.srv.URL + "/realms/canton/protocol/openid-connect/token",
		ClientID:     "otc-desk",
		ClientSecret: "secret",
		Audience:     "https://validator.example",
		Scope:        "daml_ledger_api",
	}
}

func Test_Auth_BearerTokenOnEveryCall(t *testing.T) {
	otel.SilentModeInit()

	token := jwt(t, map[string]any{"sub": "service-account-otc-desk"})
	provider := newIDP(t, token)

	var (
		mu      sync.Mutex
		headers []string
		bodies  [][]byte
	)

	ledger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		headers = append(headers, r.Header.Get("Authorization"))
		bodies = append(bodies, body)
		mu.Unlock()

		_, _ = io.WriteString(w, `{"updateId":"u-1"}`)
	}))
	t.Cleanup(ledger.Close)

	c := New(Config{URL: ledger.URL, Auth: provider.auth()})

	require.True(t, c.Ready(t.Context()))
	require.NoError(t, c.Execute(t.Context(), domain.Prepared{}, "alice::1220", []byte{1}))

	require.Equal(t, []string{"Bearer " + token, "Bearer " + token}, headers)
	require.Len(t, provider.forms, 1, "the token is cached")
	require.Equal(t, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"otc-desk"},
		"client_secret": {"secret"},
		"audience":      {"https://validator.example"},
		"scope":         {"daml_ledger_api"},
	}, provider.forms[0])

	var body struct {
		UserID string `json:"userId"`
	}
	require.NoError(t, json.Unmarshal(bodies[1], &body))
	require.Equal(t, "service-account-otc-desk", body.UserID)
}

func Test_Auth_ExplicitUserWins(t *testing.T) {
	c := New(Config{URL: "http://ledger.invalid", UserID: "desk", Auth: newIDP(t, jwt(t, map[string]any{})).auth()})

	user, err := c.user()
	require.NoError(t, err)
	require.Equal(t, "desk", user)
}

func Test_Auth_NoSubject(t *testing.T) {
	c := New(Config{URL: "http://ledger.invalid", Auth: newIDP(t, jwt(t, map[string]any{"aud": "x"})).auth()})

	_, err := c.user()
	require.ErrorIs(t, err, domain.ErrLedger)
	require.ErrorContains(t, err, "no subject")

	_, err = New(Config{URL: "http://ledger.invalid"}).user()
	require.ErrorContains(t, err, "not a JWT")
}

func Test_Auth_TokenFailureIsUnavailable(t *testing.T) {
	provider := newIDP(t, "")
	provider.status = http.StatusUnauthorized

	c := New(Config{URL: "http://ledger.invalid", UserID: "desk", Auth: provider.auth()})

	err := c.Execute(t.Context(), domain.Prepared{}, "alice::1220", nil)
	require.ErrorIs(t, err, domain.ErrLedgerUnavailable)
	require.False(t, c.Ready(t.Context()))
}

func Test_Auth_WebSocketCarriesTheToken(t *testing.T) {
	otel.SilentModeInit()

	token := jwt(t, map[string]any{"sub": "desk"})

	var protocols string

	ledger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/state/ledger-end" {
			_, _ = io.WriteString(w, `{"offset":7}`)

			return
		}

		protocols = r.Header.Get("Sec-WebSocket-Protocol")

		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	t.Cleanup(ledger.Close)

	_, err := New(Config{URL: ledger.URL, Auth: newIDP(t, token).auth()}).Holdings(t.Context(), "alice::1220")
	require.ErrorIs(t, err, domain.ErrLedgerUnavailable)
	require.Equal(t, "jwt.token."+token+",daml.ws.auth", protocols)
}
