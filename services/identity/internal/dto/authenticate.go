package dto

import "github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"

type AuthenticateInput struct {
	Token string
}

type AuthenticateOutput struct {
	User domain.User
}
