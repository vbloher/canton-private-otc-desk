package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func amount(instrument, value string) domain.Amount {
	return domain.Amount{Instrument: instrument, Value: decimal.MustParse(value)}
}

func tradeOf(direction domain.Direction) domain.Trade {
	rfq := rfqOf(direction, "0.5", 10)

	return domain.NewTrade(rfq, quoteOf(rfq, "20000"), time.Now())
}

func Test_Exchanges_WhenTheTakerBuysTheMakerPaysTheFeeOutOfWhatItReceives(t *testing.T) {
	takerSide, makerSide := tradeOf(domain.Buy).Exchanges()

	require.Equal(t, domain.Exchange{Party: taker, Gave: amount("USDCx", "10000"), Received: amount("CBTC", "0.5")}, takerSide)
	require.Equal(t, domain.Exchange{Party: makerA, Gave: amount("CBTC", "0.5"), Received: amount("USDCx", "9990")}, makerSide)
}

func Test_Exchanges_WhenTheTakerSellsTheMakerPaysTheFeeOnTopOfWhatItGives(t *testing.T) {
	takerSide, makerSide := tradeOf(domain.Sell).Exchanges()

	require.Equal(t, domain.Exchange{Party: taker, Gave: amount("CBTC", "0.5"), Received: amount("USDCx", "10000")}, takerSide)
	require.Equal(t, domain.Exchange{Party: makerA, Gave: amount("USDCx", "10010"), Received: amount("CBTC", "0.5")}, makerSide)
}
