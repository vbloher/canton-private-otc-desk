package dto

import "github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"

type AuthenticateAPIKeyInput struct {
	APIKey string
}

type AuthenticateAPIKeyOutput struct {
	User domain.User
}
