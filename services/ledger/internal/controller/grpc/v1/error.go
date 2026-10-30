package v1

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func grpcError(ctx context.Context, err error) error {
	var funds domain.InsufficientFundsError

	switch {
	case errors.As(err, &funds):
		_ = render.Error(ctx, err, "request failed")

		return status.Error(codes.FailedPrecondition, funds.Error())
	case domain.IsInvalidInput(err):
		return status.Error(codes.InvalidArgument, render.Error(ctx, err, "request failed").Error())
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, render.Error(ctx, err, "request failed").Error())
	case errors.Is(err, domain.ErrTopologyFrozen):
		_ = render.Error(ctx, err, "request failed")

		return status.Error(codes.FailedPrecondition, domain.ErrTopologyFrozen.Error())
	case errors.Is(err, domain.ErrContractGone):
		return status.Error(codes.Aborted, render.Error(ctx, err, "the ledger changed meanwhile, retry").Error())
	default:
		_ = render.Error(ctx, err, "internal error")

		return status.Error(codes.Internal, http.StatusText(http.StatusInternalServerError))
	}
}
