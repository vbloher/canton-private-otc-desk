package config

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/adapter/gateway"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/controller/worker"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/usecase"
)

type Config struct {
	Logger  logger.Config
	OTEL    otel.Config
	Gateway gateway.Config
	UseCase usecase.Config
	Worker  worker.Config
}

func New() (Config, error) {
	config := Config{
		OTEL: otel.Config{ServiceName: "maker-bot"},
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
