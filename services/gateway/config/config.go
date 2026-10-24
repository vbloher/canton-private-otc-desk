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
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/adapter/identity"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/http"
)

type Config struct {
	HTTP     httpserver.Config
	Logger   logger.Config
	OTEL     otel.Config
	Redis    redis.Config
	Identity identity.Config
	Gateway  http.Config
}

func New() (Config, error) {
	// Ports and names that belong to this service; the environment overrides them.
	config := Config{
		HTTP: httpserver.Config{Port: "8080"},
		OTEL: otel.Config{ServiceName: "gateway"},
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
