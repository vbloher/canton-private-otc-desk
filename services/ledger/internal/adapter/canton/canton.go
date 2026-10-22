// Package canton is the ledger service's only door to Canton: the JSON Ledger API v2 over
// HTTP, and its WebSocket for active-contract reads (the HTTP list endpoints cap at 200
// items). Sandbox only: the TestTokenV2 TokenRules contract plays the token registry.
package canton

import (
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Token-standard interfaces and the sandbox rail's templates, addressed by package name.
const (
	holdingV1 = "#splice-api-token-holding-v1:Splice.Api.Token.HoldingV1:Holding"

	transferInstructionModule = "#splice-api-token-transfer-instruction-v1:Splice.Api.Token.TransferInstructionV1:"
	transferInstructionV1     = transferInstructionModule + "TransferInstruction"
	transferFactoryV1         = transferInstructionModule + "TransferFactory"

	allocationModule    = "#splice-api-token-allocation-v2:Splice.Api.Token.AllocationV2:"
	allocationV2        = allocationModule + "Allocation"
	settlementFactoryV2 = allocationModule + "SettlementFactory"

	allocationInstructionModule = "#splice-api-token-allocation-instruction-v2:Splice.Api.Token.AllocationInstructionV2:"
	allocationFactoryV2         = allocationInstructionModule + "AllocationFactory"

	tokenRulesTemplate = "#splice-test-token-v2:Splice.Testing.Tokens.TestTokenV2:TokenRules"
	tokenTemplate      = "#splice-test-token-v2:Splice.Testing.Tokens.TestTokenV2.Holding:Token"

	// rulesContextKey is where TestTokenV2 looks for its rules in a choice context.
	rulesContextKey = "testTokenV2/tokenRules"
)

type Config struct {
	URL    string `env:"CANTON_URL"     envDefault:"http://localhost:6864"`
	UserID string `env:"CANTON_USER_ID" envDefault:"participant_admin"`
}

type Canton struct {
	url         string
	wsURL       string
	userID      string
	http        *http.Client
	idleTimeout time.Duration // the longest silence an active-contracts stream may keep
}

func New(c Config) *Canton {
	url := strings.TrimRight(c.URL, "/")

	return &Canton{
		url:         url,
		wsURL:       "ws" + strings.TrimPrefix(url, "http"),
		userID:      c.UserID,
		http:        &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
		idleTimeout: readTimeout,
	}
}
