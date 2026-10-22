package dto

import (
	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

type SetMakerModeInput struct {
	UserID  uuid.UUID
	Enabled bool
}

type SetMakerModeOutput struct {
	User domain.User
}
