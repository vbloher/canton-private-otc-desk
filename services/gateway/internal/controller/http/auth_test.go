package http_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase/mocks"
)

var (
	bob   = principal.Principal{UserID: uuid.New(), PartyID: "bob-77aa01::1220cd", Hint: "bob-77aa01"}
	maker = principal.Principal{UserID: uuid.New(), PartyID: "makerA::1220ef", Hint: "makerA"}
)

func withBearer(token string) func(*http.Request) {
	return withHeader("Authorization", "Bearer "+token)
}

// A frontend on another origin cannot use the session cookie, so it sends the same secret as a bearer token.
func Test_Bearer_AuthenticatesLikeTheCookie(t *testing.T) {
	tests := map[string]string{
		"canonical":          "Bearer alice-session",
		"scheme is any case": "bearer alice-session",
		"upper case scheme":  "BEARER alice-session",
		"extra spaces":       "Bearer    alice-session",
	}

	for name, header := range tests {
		t.Run(name, func(t *testing.T) {
			g := newGateway(t, identityKnowsAlice(t))

			got := echo(t, do(t, g, http.MethodGet, "/api/v1/rfqs/mine", "", withHeader("Authorization", header)))

			require.Equal(t, tradingName, got.Upstream)
			require.Equal(t, []string{alice.PartyID}, got.Header.Values(principal.HeaderPartyID))
		})
	}
}

func Test_Bearer_TokenAsIdentityIssuesIt(t *testing.T) {
	// base64url without padding, as identity generates session tokens; padding is legal in a bearer token too.
	for _, token := range []string{"Zm9vLWJhcl9iYXo-Xy0tOTk", "abc==", "a.b~c+d/e"} {
		t.Run(token, func(t *testing.T) {
			idp := mocks.NewIdentity(t)
			idp.EXPECT().Authenticate(Any, token).Return(alice, nil)

			g := newGateway(t, idp)

			got := echo(t, do(t, g, http.MethodGet, "/api/v1/trades", "", withBearer(token)))
			require.Equal(t, []string{alice.PartyID}, got.Header.Values(principal.HeaderPartyID))
		})
	}
}

// X-API-Key > bearer > cookie. A different principal sits behind every credential, so the party the service
// sees tells which one was used; the strict mock fails the test if a losing credential is looked up as well.
func Test_Bearer_Precedence(t *testing.T) {
	tests := map[string]struct {
		mutate  []func(*http.Request)
		arrange func(*mocks.Identity)
		want    principal.Principal
	}{
		"api key beats bearer and cookie": {
			mutate: []func(*http.Request){
				withHeader(domain.APIKeyHeader, "the-key"), withBearer("the-bearer"), withSession("the-cookie"),
			},
			arrange: func(m *mocks.Identity) { m.EXPECT().AuthenticateAPIKey(Any, "the-key").Return(maker, nil) },
			want:    maker,
		},
		"api key beats bearer": {
			mutate:  []func(*http.Request){withHeader(domain.APIKeyHeader, "the-key"), withBearer("the-bearer")},
			arrange: func(m *mocks.Identity) { m.EXPECT().AuthenticateAPIKey(Any, "the-key").Return(maker, nil) },
			want:    maker,
		},
		"bearer beats cookie": {
			mutate:  []func(*http.Request){withSession("the-cookie"), withBearer("the-bearer")},
			arrange: func(m *mocks.Identity) { m.EXPECT().Authenticate(Any, "the-bearer").Return(alice, nil) },
			want:    alice,
		},
		"bearer alone": {
			mutate:  []func(*http.Request){withBearer("the-bearer")},
			arrange: func(m *mocks.Identity) { m.EXPECT().Authenticate(Any, "the-bearer").Return(alice, nil) },
			want:    alice,
		},
		"cookie alone": {
			mutate:  []func(*http.Request){withSession("the-cookie")},
			arrange: func(m *mocks.Identity) { m.EXPECT().Authenticate(Any, "the-cookie").Return(bob, nil) },
			want:    bob,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			idp := mocks.NewIdentity(t)
			tt.arrange(idp)

			g := newGateway(t, idp)

			got := echo(t, do(t, g, http.MethodGet, "/api/v1/trades", "", tt.mutate...))
			require.Equal(t, []string{tt.want.PartyID}, got.Header.Values(principal.HeaderPartyID))
		})
	}
}

// Anything that is not a well-formed "Bearer <token>" is as good as no header: never a 400 and never an
// attempt to look the junk up, and a cookie next to it still works.
func Test_Bearer_MalformedCountsAsAbsent(t *testing.T) {
	malformed := map[string]string{
		"no token":              "Bearer",
		"empty token":           "Bearer ",
		"only spaces":           "Bearer    ",
		"another scheme":        "Basic dXNlcjpwYXNz",
		"unknown scheme":        "Token alice-session",
		"no scheme":             "alice-session",
		"no space after scheme": "Bearerr-alice-session",
		"tab as separator":      "Bearer\talice-session",
		"token with a space":    "Bearer alice session",
		"quoted token":          `Bearer "alice-session"`,
		"auth params":           `Bearer realm="desk", error="invalid_token"`,
		"comma in token":        "Bearer alice,session",
		"only padding":          "Bearer =",
		"padding in the middle": "Bearer ab=cd",
	}

	for name, header := range malformed {
		t.Run(name+"/alone", func(t *testing.T) {
			// No expectations: identity must not be asked about junk.
			g := newGateway(t, mocks.NewIdentity(t))

			rec := do(t, g, http.MethodGet, "/api/v1/trades", "", withHeader("Authorization", header))

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			require.JSONEq(t, `{"error":"unauthorized"}`, rec.Body.String())
			require.Zero(t, g.totalHits())
		})

		t.Run(name+"/cookie still works", func(t *testing.T) {
			idp := mocks.NewIdentity(t)
			idp.EXPECT().Authenticate(Any, "alice-session").Return(alice, nil)

			g := newGateway(t, idp)

			got := echo(t, do(t, g, http.MethodGet, "/api/v1/trades", "", withHeader("Authorization", header), withSession("alice-session")))
			require.Equal(t, []string{alice.PartyID}, got.Header.Values(principal.HeaderPartyID))
		})
	}
}

// Identity ends a session by a bearer token only when there is exactly one Authorization header, so the edge
// must agree: a request that states its credentials twice states none.
func Test_Bearer_TwoAuthorizationHeadersCountAsAbsent(t *testing.T) {
	twice := []func(*http.Request){withBearer("alice-session"), withBearer("bob-session")}

	t.Run("alone", func(t *testing.T) {
		g := newGateway(t, mocks.NewIdentity(t))

		rec := do(t, g, http.MethodGet, "/api/v1/trades", "", twice...)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, `{"error":"unauthorized"}`, rec.Body.String())
		require.Zero(t, g.totalHits())
	})

	t.Run("cookie still works", func(t *testing.T) {
		idp := mocks.NewIdentity(t)
		idp.EXPECT().Authenticate(Any, "cookie-token").Return(bob, nil)

		g := newGateway(t, idp)

		got := echo(t, do(t, g, http.MethodGet, "/api/v1/trades", "", append(twice, withSession("cookie-token"))...))
		require.Equal(t, []string{bob.PartyID}, got.Header.Values(principal.HeaderPartyID))
	})
}

func Test_Bearer_RejectedTokenIsUnauthorized(t *testing.T) {
	idp := mocks.NewIdentity(t)
	idp.EXPECT().Authenticate(Any, "logged-out").Return(principal.Principal{}, domain.ErrUnauthorized)

	g := newGateway(t, idp)

	rec := do(t, g, http.MethodGet, "/api/v1/me", "", withBearer("logged-out"), withSession("alice-session"))

	require.Equal(t, http.StatusUnauthorized, rec.Code, "a bad bearer token is not rescued by a cookie next to it")
	require.JSONEq(t, `{"error":"unauthorized"}`, rec.Body.String())
}
