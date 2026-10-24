package dto

// LogoutInput holds the session tokens a request presents: the cookie's and the bearer token, which
// may differ. An empty token is no token.
type LogoutInput struct {
	Tokens []string
}
