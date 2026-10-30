package v1

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func Test_grpcError(t *testing.T) {
	short := domain.InsufficientFundsError{Instrument: "CBTC", Need: decimal.MustParse("1"), Held: decimal.MustParse("0.5")}

	for name, tc := range map[string]struct {
		err     error
		code    codes.Code
		message string
	}{
		"insufficient funds": {
			fmt.Errorf("u.allocate: %w", short), codes.FailedPrecondition,
			"insufficient funds: CBTC: need 1.0, held 0.5",
		},
		"invalid input keeps its reason": {
			fmt.Errorf("u.legs: %w", domain.Errorf(domain.ErrValidation, "leg %q: amount %q is not a positive decimal", "base", "0")),
			codes.InvalidArgument, `request failed: validation failed: leg "base": amount "0" is not a positive decimal`,
		},
		"not found": {
			domain.Errorf(domain.ErrNotFound, "settlement s-1 has no allocation for alice|operator|CBTC"),
			codes.NotFound, "request failed: not found: settlement s-1 has no allocation for alice|operator|CBTC",
		},
		"a contract consumed meanwhile": {fmt.Errorf("canton.Submit: %w", fmt.Errorf("%w: %w: CONTRACT_NOT_FOUND",
			domain.ErrLedger, domain.ErrContractGone)), codes.Aborted, "retry"},
		"topology frozen": {
			fmt.Errorf("canton.AllocateExternalParty: %w", fmt.Errorf("%w: %w: frozen", domain.ErrLedger, domain.ErrTopologyFrozen)),
			codes.FailedPrecondition, "the network is being upgraded and does not accept new parties right now",
		},
		"ledger failure": {
			fmt.Errorf("canton.Submit: %w", errors.Join(domain.ErrLedger)), codes.Internal,
			"Internal Server Error",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := status.Convert(grpcError(t.Context(), tc.err))
			require.Equal(t, tc.code, got.Code())
			require.Contains(t, got.Message(), tc.message)
		})
	}
}
