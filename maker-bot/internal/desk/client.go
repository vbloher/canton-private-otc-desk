package desk

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"

	identityapi "github.com/vbloher/canton-private-otc-desk/maker-bot/internal/deskapi/identity"
	tradingapi "github.com/vbloher/canton-private-otc-desk/maker-bot/internal/deskapi/trading"
	walletapi "github.com/vbloher/canton-private-otc-desk/maker-bot/internal/deskapi/wallet"
)

const apiPrefix = "/api/v1"

type signer struct {
	partyID string
	sign    func(message []byte) []byte
}

type Client struct {
	streamURL string
	dialer    *http.Client
	session   *session

	identity *identityapi.ClientWithResponses
	trading  *tradingapi.ClientWithResponses
	wallet   *walletapi.ClientWithResponses

	mu     sync.Mutex
	signer *signer
}

func New(baseURL string, httpClient *http.Client) (*Client, error) {
	base := strings.TrimRight(baseURL, "/")

	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	c := &Client{
		streamURL: "ws" + strings.TrimPrefix(base, "http") + apiPrefix + "/ws",
		dialer:    &http.Client{Transport: transport},
		session:   &session{base: transport},
	}
	c.session.renew = c.SignIn

	api := &http.Client{Transport: c.session, Timeout: httpClient.Timeout}

	var err error

	c.identity, err = identityapi.NewClientWithResponses(base+apiPrefix, identityapi.WithHTTPClient(api))
	if err != nil {
		return nil, fmt.Errorf("identityapi.NewClientWithResponses: %w", err)
	}

	c.trading, err = tradingapi.NewClientWithResponses(base+apiPrefix, tradingapi.WithHTTPClient(api))
	if err != nil {
		return nil, fmt.Errorf("tradingapi.NewClientWithResponses: %w", err)
	}

	c.wallet, err = walletapi.NewClientWithResponses(base+apiPrefix, walletapi.WithHTTPClient(api))
	if err != nil {
		return nil, fmt.Errorf("walletapi.NewClientWithResponses: %w", err)
	}

	return c, nil
}

func (c *Client) SetSigner(partyID string, sign func(message []byte) []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.signer = &signer{partyID: partyID, sign: sign}
}

func (c *Client) currentSigner() (*signer, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.signer, c.signer != nil
}

func encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func decode(field, s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%s is not base64: %w", field, err)
	}

	return b, nil
}

func decimal(field, s string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("%s %q is not a decimal", field, s)
	}

	return r, nil
}
