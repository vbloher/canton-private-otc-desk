package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Maker is one house maker the bot quotes for: its API key and its spread.
type Maker struct {
	APIKey    string
	SpreadBps int64
}

// Makers is MAKER_KEYS as "api-key:spread-bps,…".
type Makers []Maker

func (m *Makers) UnmarshalText(text []byte) error {
	var makers Makers

	for entry := range strings.SplitSeq(string(text), ",") {
		key, spread, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok || key == "" {
			return fmt.Errorf("%w: maker %q is not API_KEY:SPREAD_BPS", ErrInvalidConfig, entry)
		}

		bps, err := strconv.ParseInt(spread, 10, 64)
		if err != nil || bps < 0 || bps >= bpsOne {
			return fmt.Errorf("%w: spread must be 0..9999 bps, got %q", ErrInvalidConfig, spread)
		}

		makers = append(makers, Maker{APIKey: key, SpreadBps: bps})
	}

	*m = makers

	return nil
}
