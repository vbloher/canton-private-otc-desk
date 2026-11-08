package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"

	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/kafka"
	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
	notificationredis "github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/redis"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/resend"
	kafkacontroller "github.com/vbloher/canton-private-otc-desk/services/notification/internal/controller/kafka"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/controller/worker"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
)

var ErrInvalid = errors.New("invalid config")

type Config struct {
	HTTP      httpserver.Config
	Logger    logger.Config
	OTEL      otel.Config
	Postgres  postgres.Config
	Redis     redis.Config
	Kafka     kafka.Config
	Consumer  kafkacontroller.Config
	Email     resend.Config
	Limits    notificationredis.Config
	UseCase   usecase.Config
	Worker    worker.Config
	LogEmails bool `env:"EMAIL_LOG" envDefault:"false"`
}

func New() (Config, error) {
	config := Config{
		HTTP: httpserver.Config{Port: "8084"},
		OTEL: otel.Config{ServiceName: "notification"},
	}

	err := godotenv.Load(".env")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return config, fmt.Errorf("godotenv.Load: %w", err)
	}

	err = env.Parse(&config)
	if err != nil {
		return config, fmt.Errorf("env.Parse: %w", err)
	}

	config.UseCase.Enabled = config.LogEmails || config.Email.APIKey != ""

	err = config.validate()
	if err != nil {
		return config, err
	}

	return config, nil
}

func (c Config) validate() error {
	if c.UseCase.ConfirmationTTL <= 0 {
		return fmt.Errorf("%w: EMAIL_CONFIRMATION_TTL must be positive", ErrInvalid)
	}

	if c.Limits.DailyLimit <= 0 {
		return fmt.Errorf("%w: EMAIL_CONFIRMATIONS_DAILY_LIMIT must be positive", ErrInvalid)
	}

	if !c.UseCase.Enabled {
		return nil
	}

	if !absolute(c.UseCase.PublicURL) {
		return fmt.Errorf("%w: PUBLIC_URL must be the desk's address, like https://desk.example.com", ErrInvalid)
	}

	if !c.LogEmails && c.Email.From == "" {
		return fmt.Errorf("%w: EMAIL_FROM is required with RESEND_API_KEY", ErrInvalid)
	}

	return nil
}

func absolute(raw string) bool {
	parsed, err := url.Parse(raw)

	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
