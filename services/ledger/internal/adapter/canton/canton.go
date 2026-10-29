package canton

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const (
	holdingV1             = "#splice-api-token-holding-v1:Splice.Api.Token.HoldingV1:Holding"
	transferInstructionV1 = "#splice-api-token-transfer-instruction-v1:Splice.Api.Token.TransferInstructionV1:" +
		"TransferInstruction"
	allocationV2 = "#splice-api-token-allocation-v2:Splice.Api.Token.AllocationV2:Allocation"

	tokenTimeout = 15 * time.Second
)

type Config struct {
	URL    string `env:"CANTON_URL,notEmpty"`
	UserID string `env:"CANTON_USER_ID"`
	Auth   AuthConfig
}

type AuthConfig struct {
	TokenURL     string `env:"CANTON_AUTH_TOKEN_URL"`
	ClientID     string `env:"CANTON_AUTH_CLIENT_ID"`
	ClientSecret string `env:"CANTON_AUTH_CLIENT_SECRET"`
	Audience     string `env:"CANTON_AUTH_AUDIENCE"`
	Scope        string `env:"CANTON_AUTH_SCOPE"`
}

type Canton struct {
	url         string
	wsURL       string
	userID      string
	tokens      oauth2.TokenSource
	http        *http.Client
	idleTimeout time.Duration
}

func New(c Config) *Canton {
	base := strings.TrimRight(c.URL, "/")
	transport := otelhttp.NewTransport(http.DefaultTransport)

	return &Canton{
		url:         base,
		wsURL:       "ws" + strings.TrimPrefix(base, "http"),
		userID:      c.UserID,
		tokens:      tokenSource(c.Auth, transport),
		http:        &http.Client{Transport: transport},
		idleTimeout: readTimeout,
	}
}

func tokenSource(c AuthConfig, transport http.RoundTripper) oauth2.TokenSource {
	if c.TokenURL == "" {
		return nil
	}

	params := url.Values{}
	if c.Audience != "" {
		params.Set("audience", c.Audience)
	}

	config := clientcredentials.Config{
		ClientID:       c.ClientID,
		ClientSecret:   c.ClientSecret,
		TokenURL:       c.TokenURL,
		Scopes:         strings.Fields(c.Scope),
		EndpointParams: params,
		AuthStyle:      oauth2.AuthStyleInParams,
	}

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient,
		&http.Client{Timeout: tokenTimeout, Transport: transport})

	return config.TokenSource(ctx)
}

func (c *Canton) token() (string, error) {
	if c.tokens == nil {
		return "", nil
	}

	t, err := c.tokens.Token()
	if err != nil {
		return "", unavailable(fmt.Errorf("access token: %w", err))
	}

	return t.AccessToken, nil
}

func (c *Canton) user() (string, error) {
	if c.userID != "" {
		return c.userID, nil
	}

	token, err := c.token()
	if err != nil {
		return "", err
	}

	sub, err := subject(token)
	if err != nil {
		return "", fmt.Errorf("%w: CANTON_USER_ID is not set: %w", domain.ErrLedger, err)
	}

	return sub, nil
}

func subject(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("the access token is not a JWT")
	}

	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("the access token payload: %w", err)
	}

	var claims struct {
		Sub string `json:"sub"`
	}

	err = json.Unmarshal(data, &claims)
	if err != nil {
		return "", fmt.Errorf("the access token payload: %w", err)
	}

	if claims.Sub == "" {
		return "", errors.New("the access token has no subject")
	}

	return claims.Sub, nil
}
