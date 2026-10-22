package ledger

import (
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

const insufficientFunds = "insufficient funds"

// wrap maps a LedgerV1 error onto the domain: FailedPrecondition "insufficient funds…" is
// ErrInsufficientFunds, InvalidArgument is ErrValidation, anything else is ErrUpstream.
func wrap(method string, err error) error {
	st, ok := status.FromError(err)
	if ok {
		switch {
		case st.Code() == codes.FailedPrecondition && strings.HasPrefix(st.Message(), insufficientFunds):
			detail := strings.TrimLeft(strings.TrimPrefix(st.Message(), insufficientFunds), ": ")
			if detail == "" {
				return domain.ErrInsufficientFunds
			}

			return domain.Errorf(domain.ErrInsufficientFunds, "%s", detail)
		case st.Code() == codes.InvalidArgument:
			return domain.Errorf(domain.ErrValidation, "%s", st.Message())
		}
	}

	return fmt.Errorf("%w: ledger %s: %w", domain.ErrUpstream, method, err)
}
