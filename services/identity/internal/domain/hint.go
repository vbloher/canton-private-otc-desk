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

func NewHint(displayName string) string {
	return Hintify(displayName) + "-" + hex.EncodeToString(randomBytes(hintSuffixBytes))
}
