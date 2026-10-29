package identity

import (
	"fmt"

	"github.com/google/uuid"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

func toPrincipal(user *pb.User) (principal.Principal, error) {
	userID, err := uuid.Parse(user.GetId())
	if err != nil {
		return principal.Principal{}, fmt.Errorf("%w: id %q: %w", domain.ErrInvalidUser, user.GetId(), err)
	}

	if user.GetPartyId() == "" || user.GetHint() == "" {
		return principal.Principal{}, fmt.Errorf("%w: party id and hint are required", domain.ErrInvalidUser)
	}

	return principal.Principal{
		UserID:  userID,
		PartyID: user.GetPartyId(),
		Hint:    user.GetHint(),
	}, nil
}
