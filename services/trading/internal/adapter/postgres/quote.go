package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

const quoteColumns = `id, rfq_id, maker_party, maker_hint, price::text, valid_until, status, created_at, opened_at`

type quoteRow struct {
	ID         uuid.UUID
	RFQID      uuid.UUID
	MakerParty string
	MakerHint  string
	Price      string
	ValidUntil time.Time
	Status     string
	CreatedAt  time.Time
	OpenedAt   *time.Time
}

func (r *quoteRow) dest() []any {
	return []any{
		&r.ID, &r.RFQID, &r.MakerParty, &r.MakerHint, &r.Price, &r.ValidUntil, &r.Status, &r.CreatedAt, &r.OpenedAt,
	}
}

func (r *quoteRow) toDomain() (domain.Quote, error) {
	price, err := decimal.Parse(r.Price)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("decimal.Parse price: %w", err)
	}

	return domain.Quote{
		ID:         r.ID,
		RFQID:      r.RFQID,
		Maker:      domain.Party{ID: r.MakerParty, Hint: r.MakerHint},
		Price:      price,
		ValidUntil: r.ValidUntil,
		Status:     domain.QuoteStatus(r.Status),
		CreatedAt:  r.CreatedAt,
		OpenedAt:   r.OpenedAt,
	}, nil
}

func collectQuotes(rows pgx.Rows) ([]domain.Quote, error) {
	defer rows.Close()

	var quotes []domain.Quote

	for rows.Next() {
		var r quoteRow

		err := rows.Scan(r.dest()...)
		if err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}

		quote, err := r.toDomain()
		if err != nil {
			return nil, fmt.Errorf("r.toDomain: %w", err)
		}

		quotes = append(quotes, quote)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}

	return quotes, nil
}

const getQuoteSQL = `SELECT ` + quoteColumns + ` FROM quotes WHERE id = $1`

func (p *Postgres) getQuote(ctx context.Context, sql string, id uuid.UUID) (domain.Quote, error) {
	var r quoteRow

	txOrPool := p.tx.TryExtractTX(ctx)

	err := txOrPool.QueryRow(ctx, sql, id).Scan(r.dest()...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Quote{}, domain.ErrNotFound
		}

		return domain.Quote{}, fmt.Errorf("txOrPool.QueryRow: %w", err)
	}

	quote, err := r.toDomain()
	if err != nil {
		return domain.Quote{}, fmt.Errorf("r.toDomain: %w", err)
	}

	return quote, nil
}
