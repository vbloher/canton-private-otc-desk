package domain

import (
	"encoding/hex"
	"strings"
)

const (
	hintMaxLen      = 24
	hintFallback    = "user"
	hintSuffixBytes = 3
)

// Hintify turns a display name into a conservative party hint: lower-case [a-z0-9] words joined
// by single dashes, at most 24 characters, "user" when nothing usable is left.
func Hintify(displayName string) string {
	var slug strings.Builder

	separator := false

	for _, r := range strings.ToLower(displayName) {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			separator = true

			continue
		}

		if separator && slug.Len() > 0 {
			slug.WriteByte('-')
		}

		separator = false

		slug.WriteRune(r)
	}

	hint := slug.String()
	if len(hint) > hintMaxLen {
		hint = hint[:hintMaxLen]
	}

	hint = strings.TrimRight(hint, "-")
	if hint == "" {
		return hintFallback
	}

	return hint
}

// NewHint is the party hint of a new wallet: the display name's slug plus six random hex digits,
// which keeps hints unique without a round trip.
func NewHint(displayName string) string {
	return Hintify(displayName) + "-" + hex.EncodeToString(randomBytes(hintSuffixBytes))
}
