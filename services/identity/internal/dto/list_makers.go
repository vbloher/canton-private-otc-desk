package dto

import (
	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

type ListMakersInput struct {
	CallerID uuid.UUID
}

type ListMakersOutput struct {
	Makers []domain.User
}
