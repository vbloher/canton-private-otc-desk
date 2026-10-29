package domain

// A caller proves who it is with the session cookie identity sets on sign-in (browsers)
// or with an API key (bots).
const (
	SessionCookie = "otc_session"
	APIKeyHeader  = "X-API-Key" //nolint:gosec // G101: the name of a header, not a credential
)
