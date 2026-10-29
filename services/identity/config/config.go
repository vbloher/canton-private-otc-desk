package config

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"

	"github.com/vbloher/canton-private-otc-desk/pkg/grpcserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/adapter/ledger"
	httpcontroller "github.com/vbloher/canton-private-otc-desk/services/identity/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
)

const minSessionTTL = time.Minute

var ErrInvalid = errors.New("invalid config")

type Config struct {
	HTTP     httpserver.Config
	GRPC     grpcserver.Config
	Router   httpcontroller.Config
	Logger   logger.Config
	OTEL     otel.Config
	Postgres postgres.Config
	Redis    redis.Config
	Ledger   ledger.Config
	UseCase  usecase.Config
}

func New() (Config, error) {
	config := Config{
		HTTP: httpserver.Config{Port: "8081"},
		GRPC: grpcserver.Config{Port: "9081"},
		OTEL: otel.Config{ServiceName: "identity"},
	}

	err := godotenv.Load(".env")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return config, fmt.Errorf("godotenv.Load: %w", err)
	}

	err = env.Parse(&config)
	if err != nil {
		return config, fmt.Errorf("env.Parse: %w", err)
	}

	if config.UseCase.SessionTTL < minSessionTTL {
		return config, fmt.Errorf("%w: SESSION_TTL must be at least %s", ErrInvalid, minSessionTTL)
	}

	maker := config.UseCase
	if (maker.MakerParty == "") != (maker.MakerAPIKey == "") {
		return config, fmt.Errorf("%w: MAKER_PARTY and MAKER_API_KEY go together", ErrInvalid)
	}

	if maker.MakerParty != "" && !strings.Contains(maker.MakerParty, "::") {
		return config, fmt.Errorf("%w: MAKER_PARTY must be a full party id", ErrInvalid)
	}

	return config, nil
}
