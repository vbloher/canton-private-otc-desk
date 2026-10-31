package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
)

const scope = "github.com/vbloher/canton-private-otc-desk/pkg/logger"

type Config struct {
	Level         string `env:"LOGGER_LEVEL"          envDefault:"info"`
	PrettyConsole bool   `env:"LOGGER_PRETTY_CONSOLE" envDefault:"false"`
}

func Init(c Config) {
	zerolog.TimeFieldFormat = time.RFC3339

	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	level, err := zerolog.ParseLevel(c.Level)
	if err == nil && level != zerolog.NoLevel {
		zerolog.SetGlobalLevel(level)
	}

	var out io.Writer = os.Stderr

	if c.PrettyConsole {
		out = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}
	}

	w := zerolog.MultiLevelWriter(out, NewOTelWriter(otel.Logger(scope)))

	log.Logger = zerolog.New(w).With().Timestamp().Logger().Hook(TraceHook{})

	log.Info().Msg("logger initialized")
}
