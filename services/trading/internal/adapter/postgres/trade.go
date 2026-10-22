package postgres

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

const tradeColumns = `id, rfq_id, quote_id, taker_party, taker_hint, maker_party, maker_hint, direction, base, quote,
                      qty::text, price::text, notional::text, fee::text, status, reason, update_id, created_at,
                      settled_at`

type tradeRow struct {
	ID         uuid.UUID
	RFQID      uuid.UUID
	QuoteID    uuid.UUID
	TakerParty string
	TakerHint  string
	MakerParty string
	MakerHint  string
	Direction  string
	Base       string
	Quote      string
	Qty        string
	Price      string
	Notional   string
	Fee        string
	Status     string
	Reason     string
	UpdateID   string
	CreatedAt  time.Time
	SettledAt  *time.Time
}

func (r *tradeRow) dest() []any {
	return []any{
		&r.ID, &r.RFQID, &r.QuoteID, &r.TakerParty, &r.TakerHint, &r.MakerParty, &r.MakerHint, &r.Direction,
		&r.Base, &r.Quote, &r.Qty, &r.Price, &r.Notional, &r.Fee, &r.Status, &r.Reason, &r.UpdateID,
		&r.CreatedAt, &r.SettledAt,
	}
}

func (r *tradeRow) toDomain() (domain.Trade, error) {
	amounts := make([]decimal.Decimal, 0, 4)

	for _, s := range []string{r.Qty, r.Price, r.Notional, r.Fee} {
		d, err := decimal.Parse(s)
		if err != nil {
			return domain.Trade{}, fmt.Errorf("decimal.Parse: %w", err)
		}

		amounts = append(amounts, d)
	}

	return domain.Trade{
		ID:        r.ID,
		RFQID:     r.RFQID,
		QuoteID:   r.QuoteID,
		Taker:     domain.Party{ID: r.TakerParty, Hint: r.TakerHint},
		Maker:     domain.Party{ID: r.MakerParty, Hint: r.MakerHint},
		Direction: domain.Direction(r.Direction),
		Base:      r.Base,
		Quote:     r.Quote,
		Qty:       amounts[0],
		Price:     amounts[1],
		Notional:  amounts[2],
		Fee:       amounts[3],
		Status:    domain.TradeStatus(r.Status),
		Reason:    r.Reason,
		UpdateID:  r.UpdateID,
		CreatedAt: r.CreatedAt,
		SettledAt: r.SettledAt,
	}, nil
}

func collectTrades(rows pgx.Rows) ([]domain.Trade, error) {
	defer rows.Close()

	var trades []domain.Trade

	for rows.Next() {
		var r tradeRow

		err := rows.Scan(r.dest()...)
		if err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}

		trade, err := r.toDomain()
		if err != nil {
			return nil, fmt.Errorf("r.toDomain: %w", err)
		}

		trades = append(trades, trade)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}

	return trades, nil
}
