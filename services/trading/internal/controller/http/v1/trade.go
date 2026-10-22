package v1

import (
	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func toTrade(t domain.Trade) http_server.Trade {
	out := http_server.Trade{
		ID:        t.ID,
		RfqID:     t.RFQID,
		QuoteID:   t.QuoteID,
		Taker:     t.Taker.Hint,
		Maker:     t.Maker.Hint,
		Direction: http_server.Direction(t.Direction),
		Base:      t.Base,
		Quote:     t.Quote,
		Qty:       t.Qty.String(),
		Price:     t.Price.String(),
		Notional:  t.Notional.String(),
		Fee:       t.Fee.String(),
		Status:    http_server.TradeStatus(t.Status),
		CreatedAt: t.CreatedAt.UTC(),
	}

	if t.Reason != "" {
		out.Reason = &t.Reason
	}

	if t.UpdateID != "" {
		out.UpdateID = &t.UpdateID
	}

	if t.SettledAt != nil {
		settledAt := t.SettledAt.UTC()
		out.SettledAt = &settledAt
	}

	return out
}
