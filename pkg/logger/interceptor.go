package logger

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func Interceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	var ctxErr error

	ctx = context.WithValue(ctx, ContextErrKey{}, &ctxErr)

	resp, err := handler(ctx, req)

	event := log.Info()

	switch {
	case ctxErr != nil:
		event = log.Error().Err(ctxErr)
	case err != nil:
		event = log.Error().Err(err)
	}

	event.
		Ctx(ctx).
		Str("code", status.Code(err).String()).
		Str("grpc_method", info.FullMethod).
		Send()

	return resp, err
}
