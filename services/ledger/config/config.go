package config

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"

	"github.com/vbloher/canton-private-otc-desk/pkg/grpcserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/adapter/canton"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase"
)

type Config struct {
	HTTP     httpserver.Config
	GRPC     grpcserver.Config
	Logger   logger.Config
	OTEL     otel.Config
	Postgres postgres.Config
	Redis    redis.Config
	Canton   canton.Config
	Ledger   usecase.Config
}

func New() (Config, error) {
	// The service's own defaults (spec ports, service name); the environment overrides them.
	config := Config{
		HTTP: httpserver.Config{Port: "8083"},
		GRPC: grpcserver.Config{Port: "9083"},
		OTEL: otel.Config{ServiceName: "ledger"},
	}

	err := godotenv.Load(".env")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return config, fmt.Errorf("godotenv.Load: %w", err)
	}

	err = env.Parse(&config)
	if err != nil {
		return config, fmt.Errorf("env.Parse: %w", err)
	}

	return config, nil
}
