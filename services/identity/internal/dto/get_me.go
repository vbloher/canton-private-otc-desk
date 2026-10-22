package dto

import (
	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

type GetMeInput struct {
	UserID uuid.UUID
}

type GetMeOutput struct {
	User domain.User
}
