package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/usecase/mocks"
)

const Any = mock.Anything

func config() usecase.Config {
	return usecase.Config{
		Makers: domain.Makers{{APIKey: "demo-key-a", SpreadBps: 15}, {APIKey: "demo-key-b", SpreadBps: 40}},
		Prices: domain.Prices{
			"CBTC":  decimal.MustParse("100000"),
			"CETH":  decimal.MustParse("4000"),
			"USDCx": decimal.MustParse("1"),
		},
		QuoteTTL: 60 * time.Second,
	}
}

func price(s string) any {
	return mock.MatchedBy(func(d decimal.Decimal) bool { return d.String() == s })
}

func rfq(direction string, open, quoted bool) domain.RFQ {
	return domain.RFQ{ID: uuid.New(), Direction: direction, Base: "CBTC", Quote: "USDCx", Open: open, Quoted: quoted}
}

func Test_Tick_QuotesOnlyUnquotedOpenRFQs(t *testing.T) {
	otel.SilentModeInit()

	fresh := rfq("BUY", true, false)
	quoted := rfq("BUY", true, true)
	closed := rfq("BUY", false, false)
	sell := rfq("SELL", true, false)

	gateway := mocks.NewGateway(t)
	gateway.EXPECT().ListIncomingRFQs(Any, "demo-key-a").Return([]domain.RFQ{fresh, quoted, closed}, nil)
	gateway.EXPECT().ListIncomingRFQs(Any, "demo-key-b").Return([]domain.RFQ{sell}, nil)
	gateway.EXPECT().SubmitQuote(Any, "demo-key-a", fresh.ID, price("100150.0"), 60*time.Second).Return(nil).Once()
	// A 409 (another quote won the race, the RFQ just closed…) is logged and skipped.
	gateway.EXPECT().SubmitQuote(Any, "demo-key-b", sell.ID, price("99600.0"), 60*time.Second).
		Return(domain.ErrConflict).Once()

	require.NoError(t, usecase.New(config(), gateway).Tick(t.Context()))
}

func Test_Tick_OneMakerFailingDoesNotStopTheOther(t *testing.T) {
	otel.SilentModeInit()

	fresh := rfq("BUY", true, false)
	unpriced := domain.RFQ{ID: uuid.New(), Direction: "BUY", Base: "DOGE", Quote: "USDCx", Open: true}
	errDown := errors.New("gateway down")

	gateway := mocks.NewGateway(t)
	gateway.EXPECT().ListIncomingRFQs(Any, "demo-key-a").Return(nil, errDown)
	gateway.EXPECT().ListIncomingRFQs(Any, "demo-key-b").Return([]domain.RFQ{unpriced, fresh}, nil)
	gateway.EXPECT().SubmitQuote(Any, "demo-key-b", fresh.ID, price("100400.0"), 60*time.Second).Return(nil).Once()

	err := usecase.New(config(), gateway).Tick(t.Context())
	require.ErrorIs(t, err, errDown)
	require.ErrorIs(t, err, domain.ErrUnknownInstrument)
}
