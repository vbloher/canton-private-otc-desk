package identity

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

func mapError(err error) error {
	code := status.Code(err)
	if code == codes.Unauthenticated || code == codes.NotFound {
		return domain.ErrUnauthorized
	}

	return err
}
