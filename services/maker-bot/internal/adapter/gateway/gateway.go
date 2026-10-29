package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	http_client "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/client"
)

const makerHeader = "X-Api-Key"

var ErrUnexpectedStatus = errors.New("gateway: unexpected status")

type Config struct {
	URL string `env:"GATEWAY_URL" envDefault:"http://localhost:8080"`
}

type Gateway struct {
	client *http_client.ClientWithResponses
}

func New(c Config) (*Gateway, error) {
	httpClient := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   10 * time.Second,
	}

	client, err := http_client.NewClientWithResponses(strings.TrimRight(c.URL, "/")+"/api/v1",
		http_client.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("http_client.NewClientWithResponses: %w", err)
	}

	return &Gateway{client: client}, nil
}

func asMaker(apiKey string) http_client.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set(makerHeader, apiKey)

		return nil
	}
}
