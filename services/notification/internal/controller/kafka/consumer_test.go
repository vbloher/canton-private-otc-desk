package kafka_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	kafkalib "github.com/vbloher/canton-private-otc-desk/pkg/kafka"
	"github.com/vbloher/canton-private-otc-desk/pkg/outbox"
	"github.com/vbloher/canton-private-otc-desk/pkg/tradeevents"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/controller/kafka"
)

var errDown = errors.New("database is down")

type notifier struct {
	got []tradeevents.Settled
	err error
}

func (n *notifier) NotifyTradeSettled(_ context.Context, event tradeevents.Settled) error {
	n.got = append(n.got, event)

	return n.err
}

func record(t *testing.T, eventType string, payload any) kafkalib.Record {
	t.Helper()

	value, err := json.Marshal(payload)
	require.NoError(t, err)

	return kafkalib.Record{Topic: tradeevents.Topic, Value: value, Headers: map[string]string{outbox.HeaderEventType: eventType}}
}

func settled() tradeevents.Settled {
	side := tradeevents.Side{
		PartyID:  "alice::1220",
		Gave:     tradeevents.Amount{Symbol: "CC", Value: decimal.MustParse("1")},
		Received: tradeevents.Amount{Symbol: "CBTC", Value: decimal.MustParse("2")},
	}

	return tradeevents.Settled{TradeID: uuid.New(), Taker: side, Maker: side}
}

func Test_Handle_PassesASettledTradeOn(t *testing.T) {
	var n notifier

	event := settled()

	require.NoError(t, kafka.Handler(&n)(t.Context(), record(t, tradeevents.TypeSettled, event)))

	require.Len(t, n.got, 1)
	require.Equal(t, event.TradeID, n.got[0].TradeID)
}

func Test_Handle_AFailureIsRetried(t *testing.T) {
	n := notifier{err: errDown}

	err := kafka.Handler(&n)(t.Context(), record(t, tradeevents.TypeSettled, settled()))

	require.ErrorIs(t, err, errDown)
	require.NotErrorIs(t, err, kafkalib.ErrPermanent)
}

func Test_Handle_AnUnreadableEventIsPermanent(t *testing.T) {
	var n notifier

	r := record(t, tradeevents.TypeSettled, settled())
	r.Value = []byte("{not json")

	require.ErrorIs(t, kafka.Handler(&n)(t.Context(), r), kafkalib.ErrPermanent)
	require.Empty(t, n.got)
}

func Test_Handle_AnEventWithoutItsPartiesIsPermanent(t *testing.T) {
	var n notifier

	event := settled()
	event.Maker.PartyID = ""

	require.ErrorIs(t, kafka.Handler(&n)(t.Context(), record(t, tradeevents.TypeSettled, event)), kafkalib.ErrPermanent)
	require.Empty(t, n.got)
}

func Test_Handle_IgnoresTypesItDoesNotKnow(t *testing.T) {
	var n notifier

	require.NoError(t, kafka.Handler(&n)(t.Context(), record(t, "trade.renamed", struct{}{})))
	require.Empty(t, n.got)
}
