package gateway

import (
	"context"
	"fmt"

	http_client "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/client"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/domain"
)

func (g *Gateway) ListIncomingRFQs(ctx context.Context, apiKey string) ([]domain.RFQ, error) {
	ctx, span := tracer.Start(ctx, "adapter gateway ListIncomingRFQs")
	defer span.End()

	resp, err := g.client.ListIncomingRfqsWithResponse(ctx, asMaker(apiKey))
	if err != nil {
		return nil, fmt.Errorf("client.ListIncomingRfqsWithResponse: %w", err)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("%w: %s: %s", ErrUnexpectedStatus, resp.Status(), resp.Body)
	}

	rfqs := make([]domain.RFQ, 0, len(*resp.JSON200))

	for _, in := range *resp.JSON200 {
		rfqs = append(rfqs, domain.RFQ{
			ID:        in.Rfq.ID,
			Direction: string(in.Rfq.Direction),
			Base:      in.Rfq.Base,
			Quote:     in.Rfq.Quote,
			Open:      in.Rfq.Status == http_client.RfqStatusOpen,
			Quoted:    in.MyQuote != nil,
		})
	}

	return rfqs, nil
}
