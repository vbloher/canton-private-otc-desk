package v1

import (
	"encoding/base64"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

// toSignActions never returns nil: a hosted party's list is empty, not null.
func toSignActions(actions []domain.Action) []http_server.SignAction {
	out := make([]http_server.SignAction, 0, len(actions))

	for _, a := range actions {
		out = append(out, http_server.SignAction{
			ID:          a.ID,
			Purpose:     http_server.SignActionPurpose(a.Purpose),
			Description: a.Description,
			Hash:        base64.StdEncoding.EncodeToString(a.Hash),
			CreatedAt:   a.CreatedAt.UTC(),
			ExpiresAt:   a.ExpiresAt.UTC(),
		})
	}

	return out
}
