package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

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

	log.Logger = log.With().Logger()

	if c.PrettyConsole {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})
	}

	log.Logger = log.Logger.Hook(TraceHook{})

	log.Info().Msg("logger initialized")
}
