package config

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"

	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/adapter/identity"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/adapter/ledger"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/controller/worker"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/usecase"
)

type Config struct {
	HTTP     httpserver.Config
	Logger   logger.Config
	OTEL     otel.Config
	Postgres postgres.Config
	Redis    redis.Config
	Ledger   ledger.Config
	Identity identity.Config
	UseCase  usecase.Config
	Worker   worker.Config
}

func New() (Config, error) {
	config := Config{
		HTTP: httpserver.Config{Port: "8082"},
		OTEL: otel.Config{ServiceName: "trading"},
	}

	err := godotenv.Load(".env")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return config, fmt.Errorf("godotenv.Load: %w", err)
	}

	err = env.Parse(&config)
	if err != nil {
		return config, fmt.Errorf("env.Parse: %w", err)
	}

	err = config.UseCase.Validate()
	if err != nil {
		return config, fmt.Errorf("config.UseCase.Validate: %w", err)
	}

	return config, nil
}
