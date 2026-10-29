package domain

import (
	"fmt"
	"strconv"
	"strings"
)

type Spread int64

func (s *Spread) UnmarshalText(text []byte) error {
	bps, err := strconv.ParseInt(strings.TrimSpace(string(text)), 10, 64)
	if err != nil || bps < 0 || bps >= bpsOne {
		return fmt.Errorf("%w: spread must be 0..9999 bps, got %q", ErrInvalidConfig, text)
	}

	*s = Spread(bps)

	return nil
}
