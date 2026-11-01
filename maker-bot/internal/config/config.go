package config

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/big"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

const (
	nameMin   = 2
	nameMax   = 32
	spreadMax = 9_999
	ttlMin    = 10 * time.Second
	ttlMax    = 600 * time.Second
	pollMin   = time.Second
)

var ErrInvalid = errors.New("invalid config")

type Config struct {
	DeskURL       string        `env:"DESK_URL"        envDefault:"http://localhost:8080"`
	MakerName     string        `env:"MAKER_NAME"      envDefault:"OTC Demo Maker"`
	SpreadBps     int64         `env:"SPREAD_BPS"      envDefault:"15"`
	Prices        Prices        `env:"PRICES_USD"      envDefault:"CBTC:100000,USDCx:1,CC:0.15"`
	QuoteTTL      time.Duration `env:"QUOTE_TTL"       envDefault:"600s"`
	MaxSpendShare float64       `env:"MAX_SPEND_SHARE" envDefault:"0.5"`
	PollInterval  time.Duration `env:"POLL_INTERVAL"   envDefault:"10s"`
	StateFile     string        `env:"STATE_FILE"      envDefault:"state/identity.json"`
	LogLevel      slog.Level    `env:"LOG_LEVEL"       envDefault:"info"`
}

func Load() (Config, error) {
	err := godotenv.Load(".env")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("godotenv.Load: %w", err)
	}

	return Parse(env.ToMap(os.Environ()))
}

func Parse(environ map[string]string) (Config, error) {
	if environ == nil {
		environ = map[string]string{}
	}

	var c Config

	err := env.ParseWithOptions(&c, env.Options{Environment: environ})
	if err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	c.DeskURL = strings.TrimRight(c.DeskURL, "/")
	c.MakerName = strings.TrimSpace(c.MakerName)

	err = c.validate()
	if err != nil {
		return Config{}, err
	}

	return c, nil
}

func (c Config) validate() error {
	checks := []struct {
		ok   bool
		rule string
	}{
		{
			strings.HasPrefix(c.DeskURL, "http://") || strings.HasPrefix(c.DeskURL, "https://"),
			fmt.Sprintf("DESK_URL must start with http:// or https://, got %q", c.DeskURL),
		},
		{
			within(utf8.RuneCountInString(c.MakerName), nameMin, nameMax),
			fmt.Sprintf("MAKER_NAME must be %d to %d characters", nameMin, nameMax),
		},
		{
			within(c.SpreadBps, 0, spreadMax),
			fmt.Sprintf("SPREAD_BPS must be 0 to %d, got %d", spreadMax, c.SpreadBps),
		},
		{
			within(c.QuoteTTL, ttlMin, ttlMax),
			fmt.Sprintf("QUOTE_TTL must be %s to %s, got %s", ttlMin, ttlMax, c.QuoteTTL),
		},
		{
			c.MaxSpendShare > 0 && c.MaxSpendShare <= 1,
			fmt.Sprintf("MAX_SPEND_SHARE must be above 0 and at most 1, got %v", c.MaxSpendShare),
		},
		{
			c.PollInterval >= pollMin,
			fmt.Sprintf("POLL_INTERVAL must be at least %s, got %s", pollMin, c.PollInterval),
		},
	}

	for _, check := range checks {
		if !check.ok {
			return fmt.Errorf("%w: %s", ErrInvalid, check.rule)
		}
	}

	return nil
}

func within[T cmp.Ordered](v, lowest, highest T) bool {
	return v >= lowest && v <= highest
}

type Prices map[string]*big.Rat

func (p *Prices) UnmarshalText(text []byte) error {
	prices := Prices{}

	for entry := range strings.SplitSeq(string(text), ",") {
		symbol, value, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok || symbol == "" {
			return fmt.Errorf("%w: price %q is not SYMBOL:USD", ErrInvalid, entry)
		}

		if _, dup := prices[symbol]; dup {
			return fmt.Errorf("%w: %s is priced twice", ErrInvalid, symbol)
		}

		usd, ok := new(big.Rat).SetString(value)
		if !ok || usd.Sign() <= 0 {
			return fmt.Errorf("%w: the price of %s must be a positive decimal, got %q", ErrInvalid, symbol, value)
		}

		prices[symbol] = usd
	}

	*p = prices

	return nil
}
