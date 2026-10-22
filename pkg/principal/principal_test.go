package principal_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
)

func serve(t *testing.T, h http.Header) (principal.Principal, bool) {
	t.Helper()

	var (
		got principal.Principal
		ok  bool
	)

	handler := principal.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, ok = principal.From(r.Context())
	}))

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header = h
	handler.ServeHTTP(httptest.NewRecorder(), r)

	return got, ok
}

func Test_Middleware_ParsesHeaders(t *testing.T) {
	want := principal.Principal{UserID: uuid.New(), PartyID: "alice-1a2b3c::1220ab", Hint: "alice-1a2b3c"}

	h := http.Header{}
	want.SetHeaders(h)

	got, ok := serve(t, h)
	require.True(t, ok)
	require.Equal(t, want, got)
}

func Test_Middleware_MissingOrMalformed(t *testing.T) {
	_, ok := serve(t, http.Header{})
	require.False(t, ok)

	h := http.Header{}
	h.Set(principal.HeaderUserID, "not-a-uuid")
	h.Set(principal.HeaderPartyID, "p::1")
	h.Set(principal.HeaderPartyHint, "p")

	_, ok = serve(t, h)
	require.False(t, ok)

	h = http.Header{}
	h.Set(principal.HeaderUserID, uuid.NewString())

	_, ok = serve(t, h)
	require.False(t, ok)
}

func Test_StripHeaders(t *testing.T) {
	h := http.Header{}
	principal.Principal{UserID: uuid.New(), PartyID: "p::1", Hint: "p"}.SetHeaders(h)
	h.Set("X-Other", "kept")

	principal.StripHeaders(h)

	require.Empty(t, h.Get(principal.HeaderUserID))
	require.Empty(t, h.Get(principal.HeaderPartyID))
	require.Empty(t, h.Get(principal.HeaderPartyHint))
	require.Equal(t, "kept", h.Get("X-Other"))
}
