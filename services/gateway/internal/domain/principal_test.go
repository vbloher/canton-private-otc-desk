package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

func Test_IsToken68(t *testing.T) {
	tests := map[string]bool{
		// what identity issues: unpadded base64url
		"Zm9vLWJhcl9iYXo-Xy0tOTk": true,
		"abcDEF123":               true,
		// every symbol the syntax has, and padding
		"a-b.c_d~e+f/g":   true,
		"abc=":            true,
		"abc==":           true,
		"a-b.c_d~e+f/g==": true,
		"0":               true,
		// junk
		"":               false,
		"=":              false,
		"==":             false,
		"ab=cd":          false,
		"=abc":           false,
		"ab cd":          false,
		" abc":           false,
		"abc ":           false,
		"ab\tcd":         false,
		"ab,cd":          false,
		"ab;cd":          false,
		`"abc"`:          false,
		"ab:cd":          false,
		"ab\x00cd":       false,
		"tökén":          false,
		"ab\ncd":         false,
		"realm=\"desk\"": false,
	}

	for token, want := range tests {
		require.Equal(t, want, domain.IsToken68(token), "%q", token)
	}
}
