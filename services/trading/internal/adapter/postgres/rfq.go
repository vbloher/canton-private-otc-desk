package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

const rfqColumns = `id, taker_party, taker_hint, direction, qty::text, base, quote, makers, fee_bps,
                    deadline, status, created_at, closed_at`

// party is a maker as stored in rfqs.makers.
type party struct {
	ID   string `json:"id"`
	Hint string `json:"hint"`
}

func marshalMakers(makers []domain.Party) ([]byte, error) {
	rows := make([]party, 0, len(makers))
	for _, m := range makers {
		rows = append(rows, party{ID: m.ID, Hint: m.Hint})
	}

	b, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal: %w", err)
	}

	return b, nil
}

type rfqRow struct {
	ID         uuid.UUID
	TakerParty string
	TakerHint  string
	Direction  string
	Qty        string
	Base       string
	Quote      string
	Makers     []byte
	FeeBps     int64
	Deadline   time.Time
	Status     string
	CreatedAt  time.Time
	ClosedAt   *time.Time
}

func (r *rfqRow) dest() []any {
	return []any{
		&r.ID, &r.TakerParty, &r.TakerHint, &r.Direction, &r.Qty, &r.Base, &r.Quote, &r.Makers, &r.FeeBps,
		&r.Deadline, &r.Status, &r.CreatedAt, &r.ClosedAt,
	}
}

func (r *rfqRow) toDomain() (domain.RFQ, error) {
	qty, err := decimal.Parse(r.Qty)
	if err != nil {
		return domain.RFQ{}, fmt.Errorf("decimal.Parse qty: %w", err)
	}

	var rows []party

	err = json.Unmarshal(r.Makers, &rows)
	if err != nil {
		return domain.RFQ{}, fmt.Errorf("json.Unmarshal makers: %w", err)
	}

	makers := make([]domain.Party, 0, len(rows))
	for _, m := range rows {
		makers = append(makers, domain.Party{ID: m.ID, Hint: m.Hint})
	}

	return domain.RFQ{
		ID:         r.ID,
		TakerParty: r.TakerParty,
		TakerHint:  r.TakerHint,
		Direction:  domain.Direction(r.Direction),
		Qty:        qty,
		Base:       r.Base,
		Quote:      r.Quote,
		Makers:     makers,
		FeeBps:     r.FeeBps,
		Deadline:   r.Deadline,
		Status:     domain.RFQStatus(r.Status),
		CreatedAt:  r.CreatedAt,
		ClosedAt:   r.ClosedAt,
	}, nil
}

func collectRFQs(rows pgx.Rows) ([]domain.RFQ, error) {
	defer rows.Close()

	var rfqs []domain.RFQ

	for rows.Next() {
		var r rfqRow

		err := rows.Scan(r.dest()...)
		if err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}

		rfq, err := r.toDomain()
		if err != nil {
			return nil, fmt.Errorf("r.toDomain: %w", err)
		}

		rfqs = append(rfqs, rfq)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}

	return rfqs, nil
}
