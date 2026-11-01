package desk

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	tradingapi "github.com/vbloher/canton-private-otc-desk/maker-bot/internal/deskapi/trading"
)

func (c *Client) IncomingRFQs(ctx context.Context) ([]IncomingRFQ, error) {
	resp, err := c.trading.ListIncomingRfqsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("incoming rfqs: %w", err)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("incoming rfqs: %w", statusError(resp.StatusCode(), resp.Body))
	}

	out := make([]IncomingRFQ, 0, len(*resp.JSON200))

	for _, in := range *resp.JSON200 {
		qty, err := decimal("qty", in.Rfq.Qty)
		if err != nil {
			return nil, err
		}

		incoming := IncomingRFQ{RFQ: RFQ{
			ID:        in.Rfq.ID,
			TakerBuys: string(in.Rfq.Direction) == "BUY",
			Qty:       qty,
			Base:      in.Rfq.Base,
			Quote:     in.Rfq.Quote,
			FeeBps:    in.Rfq.FeeBps,
			Deadline:  in.Rfq.Deadline,
			Open:      string(in.Rfq.Status) == "open",
		}}

		if in.MyQuote != nil {
			incoming.MyQuote = &Quote{ID: in.MyQuote.ID, Status: string(in.MyQuote.Status)}
		}

		out = append(out, incoming)
	}

	return out, nil
}

func (c *Client) SubmitQuote(ctx context.Context, rfqID uuid.UUID, price string, ttl time.Duration) (Quote, error) {
	seconds := int(ttl / time.Second)

	resp, err := c.trading.SubmitQuoteWithResponse(ctx, rfqID, tradingapi.SubmitQuoteInput{
		Price:      price,
		TTLSeconds: &seconds,
	})
	if err != nil {
		return Quote{}, fmt.Errorf("submit quote: %w", err)
	}

	if resp.JSON201 == nil {
		return Quote{}, fmt.Errorf("submit quote: %w", statusError(resp.StatusCode(), resp.Body))
	}

	return Quote{ID: resp.JSON201.ID, Status: string(resp.JSON201.Status)}, nil
}
