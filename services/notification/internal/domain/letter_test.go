package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

const base = "https://desk.example.com"

func Test_FormatAmount(t *testing.T) {
	cases := map[string]string{
		"12345.0":      "12,345 CC",
		"0.5":          "0.5 CC",
		"1234567.125":  "1,234,567.125 CC",
		"999.0":        "999 CC",
		"1000.0":       "1,000 CC",
		"0.0000000001": "0.0000000001 CC",
	}

	for value, want := range cases {
		require.Equal(t, want, domain.FormatAmount(decimal.MustParse(value), "CC"))
	}
}

func Test_ConfirmationLetter_CarriesTheLink(t *testing.T) {
	link := base + "/confirm-email#token=abc"

	letter, err := domain.ConfirmationLetter(link, 24*time.Hour)

	require.NoError(t, err)
	require.Equal(t, "Confirm your email for Private OTC Desk", letter.Subject)
	require.Contains(t, letter.HTML, `href="`+link+`"`)
	require.Contains(t, letter.Text, link)
	require.Contains(t, letter.Text, "24 hours")
}

func trade(updateID string) domain.TradeFacts {
	return domain.TradeFacts{Gave: "0.5 CBTC", Received: "12,345 CC", UpdateID: updateID, BaseURL: base}
}

func Test_TradeLetter_StatesWhatWasExchanged(t *testing.T) {
	letter, err := domain.TradeLetter(trade(""))

	require.NoError(t, err)
	require.Equal(t, "Your trade settled", letter.Subject)
	require.Contains(t, letter.HTML, "You gave 0.5 CBTC and received 12,345 CC.")
	require.Contains(t, letter.Text, "You gave 0.5 CBTC and received 12,345 CC.")
	require.Contains(t, letter.HTML, base+"/activity")
	require.Contains(t, letter.Text, base+"/profile")
}

func Test_TradeLetter_WithoutAnUpdateIDHasNoSuchLine(t *testing.T) {
	letter, err := domain.TradeLetter(trade(""))

	require.NoError(t, err)
	require.NotContains(t, letter.HTML, "update id")
	require.NotContains(t, letter.Text, "update id")
}

func Test_TradeLetter_ShowsTheUpdateIDWhenGiven(t *testing.T) {
	letter, err := domain.TradeLetter(trade("1220abc"))

	require.NoError(t, err)
	require.Contains(t, letter.HTML, "Canton update id: 1220abc")
	require.Contains(t, letter.Text, "Canton update id: 1220abc")
}

func Test_TradeLetter_EscapesWhatIsInserted(t *testing.T) {
	facts := trade("")
	facts.Gave = "<b>0.5</b> CBTC"

	letter, err := domain.TradeLetter(facts)

	require.NoError(t, err)
	require.NotContains(t, letter.HTML, "<b>0.5</b>")
	require.Contains(t, letter.HTML, "&lt;b&gt;0.5&lt;/b&gt;")
}
