package gateway

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	http_client "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/client"
	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/domain"
)

func (g *Gateway) SubmitQuote(
	ctx context.Context, apiKey string, rfqID uuid.UUID, price decimal.Decimal, ttl time.Duration,
) error {
	ctx, span := tracer.Start(ctx, "adapter gateway SubmitQuote")
	defer span.End()

	ttlSeconds := int(ttl / time.Second)

	resp, err := g.client.SubmitQuoteWithResponse(ctx, rfqID, http_client.SubmitQuoteInput{
		Price:      price.String(),
		TTLSeconds: &ttlSeconds,
	}, asMaker(apiKey))
	if err != nil {
		return fmt.Errorf("client.SubmitQuoteWithResponse: %w", err)
	}

	switch resp.StatusCode() {
	case http.StatusCreated:
		return nil
	case http.StatusConflict:
		return fmt.Errorf("%w: %s", domain.ErrConflict, resp.Body)
	default:
		return fmt.Errorf("%w: %s: %s", ErrUnexpectedStatus, resp.Status(), resp.Body)
	}
}
