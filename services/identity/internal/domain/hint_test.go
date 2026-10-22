package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func Test_Hintify(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "words become a slug", input: "Alice Smith", want: "alice-smith"},
		{name: "only symbols falls back to user", input: "  !!", want: "user"},
		{name: "empty falls back to user", input: "", want: "user"},
		{name: "long names are cut to 24", input: strings.Repeat("a", 40), want: strings.Repeat("a", 24)},
		{name: "runs collapse and edges trim", input: "--Bob   &  the__Builder!!", want: "bob-the-builder"},
		{name: "digits are kept", input: "Neo 2049", want: "neo-2049"},
		{name: "non ascii letters count as separators", input: "Алиса", want: "user"},
		{name: "a cut never leaves a trailing dash", input: strings.Repeat("a", 23) + " bbb", want: strings.Repeat("a", 23)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, domain.Hintify(c.input))
		})
	}
}

func Test_NewHint_Suffix(t *testing.T) {
	hint := domain.NewHint("Alice")
	require.Regexp(t, `^alice-[0-9a-f]{6}$`, hint)

	seen := map[string]struct{}{}
	for range 20 {
		seen[domain.NewHint("Alice")] = struct{}{}
	}

	require.Greater(t, len(seen), 1, "the suffix must be random")
}

func Test_NewHint_FallsBackToUser(t *testing.T) {
	require.Regexp(t, `^user-[0-9a-f]{6}$`, domain.NewHint("!!"))
}
