//go:build e2e

package e2e

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	identity "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/client"
	trading "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/client"
	wallet "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/client"
	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

// A browser wallet buys 0.1 CBTC from makerA (quoted by the maker bot) and the swap settles atomically.
func Test_FullCycle(t *testing.T) {
	taker := newWallet(t, "E2E Taker")
	taker.fund(t)

	cbtcBefore := decimal.MustParse(taker.balance(t, "CBTC").Available)
	usdcBefore := decimal.MustParse(taker.balance(t, "USDCx").Available)

	created, err := taker.trading.CreateRfqWithResponse(t.Context(), trading.CreateRfqInput{
		Direction:  trading.DirectionBUY,
		Qty:        "0.1",
		Base:       "CBTC",
		Quote:      "USDCx",
		Makers:     []string{"makerA"},
		TTLSeconds: 300,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, created.StatusCode(), string(created.Body))

	rfqID := created.JSON201.ID

	var quote trading.Quote

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		mine, err := taker.trading.ListMyRfqsWithResponse(t.Context())
		if !assert.NoError(c, err) || !assert.Equal(c, http.StatusOK, mine.StatusCode()) {
			return
		}

		for _, r := range *mine.JSON200 {
			for _, q := range r.Quotes {
				if r.Rfq.ID == rfqID && q.Status == trading.QuoteStatusOpen {
					quote = q

					return
				}
			}
		}

		c.Errorf("no open quote on %s yet", rfqID)
	}, waitFor, tick, "makerA never quoted — is the maker bot running?")

	accepted, err := taker.trading.AcceptQuoteWithResponse(t.Context(), quote.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, accepted.StatusCode(), string(accepted.Body))
	require.Equal(t, trading.TradeStatusSettling, accepted.JSON202.Status)

	var final trading.Trade

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := taker.trySignPending(t.Context())
		assert.NoError(c, err)

		trades, err := taker.trading.ListTradesWithResponse(t.Context())
		if !assert.NoError(c, err) || !assert.Equal(c, http.StatusOK, trades.StatusCode()) {
			return
		}

		for _, trade := range *trades.JSON200 {
			if trade.QuoteID == quote.ID {
				final = trade
				// a failed trade ends the wait at once; the assertion below reports it
				assert.Contains(c, []trading.TradeStatus{trading.TradeStatusSettled, trading.TradeStatusFailed}, trade.Status)

				return
			}
		}

		c.Errorf("no trade for quote %s yet", quote.ID)
	}, waitFor, tick, "trade never settled")
	require.Equal(t, trading.TradeStatusSettled, final.Status, final.Reason)

	notional := decimal.MustParse("0.1").Mul(decimal.MustParse(quote.Price))

	require.Equal(t, cbtcBefore.Add(decimal.MustParse("0.1")).String(), taker.balance(t, "CBTC").Available)
	require.Equal(t, usdcBefore.Sub(notional).String(), taker.balance(t, "USDCx").Available)

	again, err := taker.trading.AcceptQuoteWithResponse(t.Context(), quote.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, again.StatusCode(), "a settled quote must not be accepted twice")
}

// A browser wallet withdraws to a house maker, which accepts the incoming transfer through its API key.
func Test_WithdrawToHouseMaker(t *testing.T) {
	alice := newWallet(t, "E2E Alice")
	alice.fund(t)

	makerB := newUser(t, "demo-key-b")

	me, err := makerB.identity.GetMeWithResponse(t.Context())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, me.StatusCode(), string(me.Body))

	withdrawn, err := alice.wallet.WithdrawWithResponse(t.Context(), wallet.WithdrawInput{
		Symbol:   "CETH",
		Amount:   "1.5",
		Receiver: me.JSON200.PartyID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, withdrawn.StatusCode(), string(withdrawn.Body))
	require.Len(t, withdrawn.JSON202.Actions, 1)
	require.Equal(t, 1, alice.signPending(t))

	var offer wallet.IncomingTransfer

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		incoming, err := makerB.wallet.ListIncomingWithResponse(t.Context())
		if !assert.NoError(c, err) || !assert.Equal(c, http.StatusOK, incoming.StatusCode()) {
			return
		}

		for _, in := range *incoming.JSON200 {
			if in.Sender == alice.me.Hint && in.Symbol == "CETH" {
				offer = in

				return
			}
		}

		c.Errorf("no CETH transfer from %s yet", alice.me.Hint)
	}, waitFor, tick, "makerB never saw the transfer")

	require.Equal(t, "1.5", offer.Amount)
	require.Equal(t, "8.5", alice.balance(t, "CETH").Available)

	accepted, err := makerB.wallet.AcceptIncomingWithResponse(t.Context(), offer.ContractID)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, accepted.StatusCode(), string(accepted.Body))
	require.Empty(t, accepted.JSON202.Actions, "the desk signs for a house maker")
}

func Test_Unauthenticated(t *testing.T) {
	anonymous := newUser(t, "")

	me, err := anonymous.identity.GetMeWithResponse(t.Context())
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, me.StatusCode())

	login, err := anonymous.identity.LoginWithResponse(t.Context(), identity.LoginInput{
		Challenge:    "otc-desk:forged",
		Signature:    base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)),
		CredentialID: "nobody",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, login.StatusCode(), "a challenge the desk never issued must not sign anyone in")
}
