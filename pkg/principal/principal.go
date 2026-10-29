package principal

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const (
	HeaderUserID    = "X-User-Id"
	HeaderPartyID   = "X-Party-Id"
	HeaderPartyHint = "X-Party-Hint"
)

type Principal struct {
	UserID  uuid.UUID
	PartyID string
	Hint    string
}

type ctxKey struct{}

func (p Principal) SetHeaders(h http.Header) {
	h.Set(HeaderUserID, p.UserID.String())
	h.Set(HeaderPartyID, p.PartyID)
	h.Set(HeaderPartyHint, p.Hint)
}

func StripHeaders(h http.Header) {
	h.Del(HeaderUserID)
	h.Del(HeaderPartyID)
	h.Del(HeaderPartyHint)
}

func WithContext(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)

	return p, ok
}

func Middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		p, ok := fromHeaders(r.Header)
		if ok {
			r = r.WithContext(WithContext(r.Context(), p))
		}

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func fromHeaders(h http.Header) (Principal, bool) {
	userID, err := uuid.Parse(h.Get(HeaderUserID))
	if err != nil {
		return Principal{}, false
	}

	p := Principal{
		UserID:  userID,
		PartyID: h.Get(HeaderPartyID),
		Hint:    h.Get(HeaderPartyHint),
	}

	if p.PartyID == "" || p.Hint == "" {
		return Principal{}, false
	}

	return p, true
}
