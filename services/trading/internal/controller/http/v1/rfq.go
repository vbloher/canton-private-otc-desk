package v1

import (
	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func toRfq(r domain.RFQ) http_server.Rfq {
	makers := make([]string, 0, len(r.Makers))
	for _, m := range r.Makers {
		makers = append(makers, m.Hint)
	}

	return http_server.Rfq{
		ID:        r.ID,
		Taker:     r.TakerHint,
		Direction: http_server.Direction(r.Direction),
		Qty:       r.Qty.String(),
		Base:      r.Base,
		Quote:     r.Quote,
		Makers:    makers,
		FeeBps:    int(r.FeeBps),
		Deadline:  r.Deadline.UTC(),
		Status:    http_server.RfqStatus(r.Status),
		CreatedAt: r.CreatedAt.UTC(),
	}
}
