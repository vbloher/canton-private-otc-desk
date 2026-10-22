package v1

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func grpcError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, render.Error(ctx, err, "request failed").Error())
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, render.Error(ctx, err, "request failed").Error())
	case domain.IsInvalidInput(err):
		return status.Error(codes.InvalidArgument, render.Error(ctx, err, "request failed").Error())
	default:
		_ = render.Error(ctx, err, "internal error")

		return status.Error(codes.Internal, http.StatusText(http.StatusInternalServerError))
	}
}
