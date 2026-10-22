package v1

import (
	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func toUser(u domain.User) *pb.User {
	return &pb.User{
		Id:          u.ID.String(),
		PartyId:     u.PartyID,
		Hint:        u.Hint,
		DisplayName: u.DisplayName,
		MakerMode:   u.MakerMode,
	}
}
