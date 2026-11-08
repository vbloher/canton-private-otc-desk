package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func Test_NormalizeEmail_LowercasesAndTrims(t *testing.T) {
	got, err := domain.NormalizeEmail("  Alice.Smith+desk@Example.COM ")

	require.NoError(t, err)
	require.Equal(t, "alice.smith+desk@example.com", got)
}

func Test_NormalizeEmail_Rejects(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"spaces only":       "   ",
		"no at":             "alice.example.com",
		"no domain dot":     "alice@localhost",
		"display name":      "Alice <alice@example.com>",
		"two addresses":     "a@example.com,b@example.com",
		"header injection":  "alice@example.com\r\nBcc: mallory@example.com",
		"inner space":       "al ice@example.com",
		"quoted local part": `"al ice"@example.com`,
		"too long":          strings.Repeat("a", 250) + "@example.com",
		"slash in address":  "a/b@example.com",
		"parent directory":  "../emails@example.com",
		"question mark":     "a?b@example.com",
		"hash":              "a#b@example.com",
		"percent":           "a%2fb@example.com",
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NormalizeEmail(input)

			require.ErrorIs(t, err, domain.ErrValidation)
		})
	}
}

func Test_Profile_StatusFollowsTheAddressAndItsConfirmation(t *testing.T) {
	now := time.Now()

	require.Equal(t, domain.EmailNone, domain.Profile{}.Status())
	require.Equal(t, domain.EmailUnconfirmed, domain.Profile{Email: "a@b.co"}.Status())
	require.Equal(t, domain.EmailConfirmed, domain.Profile{Email: "a@b.co", ConfirmedAt: &now}.Status())
}

func Test_Profile_WantsTradesOnlyWithAConfirmedAddress(t *testing.T) {
	now := time.Now()

	require.True(t, domain.Profile{Email: "a@b.co", ConfirmedAt: &now, Trades: true}.WantsTrades())
	require.False(t, domain.Profile{Email: "a@b.co", Trades: true}.WantsTrades())
	require.False(t, domain.Profile{Email: "a@b.co", ConfirmedAt: &now}.WantsTrades())
}

func Test_Profile_WithANewAddressDropsTheConfirmationAndKeepsPreferences(t *testing.T) {
	now := time.Now()

	got := domain.Profile{Email: "old@b.co", ConfirmedAt: &now, News: true, Trades: true}.WithEmail("new@b.co")

	require.Equal(t, domain.Profile{Email: "new@b.co", News: true, Trades: true}, got)
}

func Test_CanonicalEmail_FoldsSpellingsOfOneInbox(t *testing.T) {
	cases := map[string]string{
		"victim@gmail.com":           "victim@gmail.com",
		"victim+1@gmail.com":         "victim@gmail.com",
		"v.ic.tim@gmail.com":         "victim@gmail.com",
		"v.ictim+x@googlemail.com":   "victim@gmail.com",
		"first.last+tag@example.com": "first.last@example.com",
		"first.last@example.com":     "first.last@example.com",
		"plus+one+two@example.com":   "plus@example.com",
	}

	for input, want := range cases {
		require.Equal(t, want, domain.CanonicalEmail(input), input)
	}
}
