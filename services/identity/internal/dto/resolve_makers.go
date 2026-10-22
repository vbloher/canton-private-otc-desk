package dto

import "github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"

type ResolveMakersInput struct {
	Hints []string
}

type ResolveMakersOutput struct {
	Makers []domain.User
}
