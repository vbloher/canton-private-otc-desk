package v1

import (
	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func toQuote(q domain.Quote) http_server.Quote {
	return http_server.Quote{
		ID:         q.ID,
		RfqID:      q.RFQID,
		Maker:      q.Maker.Hint,
		Price:      q.Price.String(),
		ValidUntil: q.ValidUntil.UTC(),
		Status:     http_server.QuoteStatus(q.Status),
		CreatedAt:  q.CreatedAt.UTC(),
	}
}
