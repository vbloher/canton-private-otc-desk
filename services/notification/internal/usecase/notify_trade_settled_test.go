package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/tradeevents"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
)

func amount(value, symbol string) tradeevents.Amount {
	return tradeevents.Amount{Symbol: symbol, Value: decimal.MustParse(value)}
}

func settled() tradeevents.Settled {
	return tradeevents.Settled{
		TradeID:   uuid.New(),
		UpdateID:  "1220deadbeef",
		SettledAt: time.Now(),
		Taker:     tradeevents.Side{PartyID: alice, Gave: amount("12345", "CC"), Received: amount("0.5", "CBTC")},
		Maker:     tradeevents.Side{PartyID: bob, Gave: amount("0.5", "CBTC"), Received: amount("12332.655", "CC")},
	}
}

func wantingTrades(party, email string) domain.Profile {
	p := confirmedProfile(party, email)
	p.Trades = true

	return p
}

func (d deps) expectProfiles(profiles ...domain.Profile) {
	d.postgres.EXPECT().ListProfiles(Any, []string{alice, bob}).Return(profiles, nil)
}

func (d deps) collectEmails() *[]domain.Email {
	var emails []domain.Email

	d.postgres.EXPECT().CreateEmail(inTx(), Any).Run(func(_ context.Context, e domain.Email) {
		emails = append(emails, e)
	}).Return(nil)

	return &emails
}

func Test_NotifyTradeSettled_WritesToBothSidesWhatEachExchanged(t *testing.T) {
	d := newDeps(t)
	event := settled()

	d.expectProfiles(wantingTrades(alice, "alice@b.co"), wantingTrades(bob, "bob@b.co"))
	emails := d.collectEmails()

	require.NoError(t, d.useCase(t, testConfig).NotifyTradeSettled(t.Context(), event))

	require.Len(t, *emails, 2)

	taker, maker := (*emails)[0], (*emails)[1]
	require.Equal(t, "alice@b.co", taker.Recipient)
	require.Equal(t, "trade-settled:"+event.TradeID.String()+":"+alice, taker.DedupeKey)
	require.Equal(t, domain.KindTradeSettled, taker.Kind)
	require.Contains(t, taker.Text, "You gave 12,345 CC and received 0.5 CBTC.")
	require.Equal(t, "bob@b.co", maker.Recipient)
	require.Contains(t, maker.Text, "You gave 0.5 CBTC and received 12,332.655 CC.")
}

func Test_NotifyTradeSettled_NeverNamesEitherSide(t *testing.T) {
	d := newDeps(t)

	d.expectProfiles(wantingTrades(alice, "alice@b.co"), wantingTrades(bob, "bob@b.co"))
	emails := d.collectEmails()

	require.NoError(t, d.useCase(t, testConfig).NotifyTradeSettled(t.Context(), settled()))

	for _, e := range *emails {
		for _, name := range []string{alice, bob, "alice-3f9a1c", "bob-77aa01"} {
			require.NotContains(t, e.Subject, name)
			require.NotContains(t, e.HTML, name)
			require.NotContains(t, e.Text, name)
		}
	}
}

func Test_NotifyTradeSettled_LeavesTheUpdateIDOutByDefault(t *testing.T) {
	d := newDeps(t)

	d.expectProfiles(wantingTrades(alice, "alice@b.co"))
	emails := d.collectEmails()

	require.NoError(t, d.useCase(t, testConfig).NotifyTradeSettled(t.Context(), settled()))
	require.NotContains(t, (*emails)[0].Text, "1220deadbeef")
	require.NotContains(t, (*emails)[0].HTML, "1220deadbeef")
}

func Test_NotifyTradeSettled_ShowsTheUpdateIDWhenTheFlagIsOn(t *testing.T) {
	d := newDeps(t)
	c := testConfig
	c.IncludeUpdateID = true

	d.expectProfiles(wantingTrades(alice, "alice@b.co"))
	emails := d.collectEmails()

	require.NoError(t, d.useCase(t, c).NotifyTradeSettled(t.Context(), settled()))
	require.Contains(t, (*emails)[0].Text, "Canton update id: 1220deadbeef")
}

func Test_NotifyTradeSettled_WritesOnlyToThoseWhoAskedForIt(t *testing.T) {
	d := newDeps(t)
	unconfirmed := domain.Profile{PartyID: bob, Email: "bob@b.co", Trades: true}

	d.expectProfiles(confirmedProfile(alice, "alice@b.co"), unconfirmed)

	require.NoError(t, d.useCase(t, testConfig).NotifyTradeSettled(t.Context(), settled()))
}

func Test_NotifyTradeSettled_DoesNothingWhenEmailIsOff(t *testing.T) {
	d := newDeps(t)

	require.NoError(t, d.useCase(t, usecase.Config{}).NotifyTradeSettled(t.Context(), settled()))
}
