//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	identity "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/client"
)

// A frontend on another origin cannot use the session cookie (it would be a third-party cookie),
// so it presents the session token as a bearer token instead.
func Test_BearerSession(t *testing.T) {
	registered := newWallet(t, "E2E Bearer")

	bearer := func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+registered.session)

		return nil
	}

	// a client with no cookie jar at all
	client, err := identity.NewClientWithResponses(baseURL(),
		identity.WithHTTPClient(&http.Client{}), identity.WithRequestEditorFn(bearer))
	require.NoError(t, err)

	me, err := client.GetMeWithResponse(t.Context())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, me.StatusCode(), string(me.Body))
	require.Equal(t, registered.me.PartyID, me.JSON200.PartyID)

	logout, err := client.LogoutWithResponse(t.Context(), &identity.LogoutParams{})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, logout.StatusCode())

	after, err := client.GetMeWithResponse(t.Context())
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, after.StatusCode(), "a logged-out bearer token must not authenticate")
}
