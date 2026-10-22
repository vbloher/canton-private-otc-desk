package identity

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

// mapError turns "identity does not know this credential" into domain.ErrUnauthorized.
// Every other failure is returned as it is: the caller cannot tell a bad credential from an outage otherwise.
func mapError(err error) error {
	code := status.Code(err)
	if code == codes.Unauthenticated || code == codes.NotFound {
		return domain.ErrUnauthorized
	}

	return err
}
