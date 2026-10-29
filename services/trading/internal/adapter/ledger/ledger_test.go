package ledger_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/adapter/ledger"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

type fakeLedger struct {
	ledger_v1.UnimplementedLedgerV1Server

	allocateIn  *ledger_v1.AllocateInput
	allocateErr error
	settleErr   error
	settlements *ledger_v1.GetSettlementsOutput
}

func (f *fakeLedger) Allocate(_ context.Context, in *ledger_v1.AllocateInput) (*ledger_v1.AllocateOutput, error) {
	f.allocateIn = in

	return &ledger_v1.AllocateOutput{Executed: true}, f.allocateErr
}

func (f *fakeLedger) Settle(context.Context, *ledger_v1.SettleInput) (*ledger_v1.SettleOutput, error) {
	return &ledger_v1.SettleOutput{UpdateId: "update-1"}, f.settleErr
}

func (f *fakeLedger) GetDesk(context.Context, *ledger_v1.GetDeskInput) (*ledger_v1.GetDeskOutput, error) {
	return &ledger_v1.GetDeskOutput{
		OperatorPartyId: "operator::1220",
		Instruments:     []*ledger_v1.Instrument{{Symbol: "CBTC", Decimals: 8}, {Symbol: "USDCx", Decimals: 2}},
	}, nil
}

func (f *fakeLedger) GetSettlements(
	context.Context, *ledger_v1.GetSettlementsInput,
) (*ledger_v1.GetSettlementsOutput, error) {
	return f.settlements, nil
}

func serve(t *testing.T, fake *fakeLedger) *ledger.Ledger {
	t.Helper()

	otel.SilentModeInit()

	lis, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	ledger_v1.RegisterLedgerV1Server(server, fake)

	go func() { _ = server.Serve(lis) }()

	t.Cleanup(server.Stop)

	client, err := ledger.New(ledger.Config{Addr: lis.Addr().String()})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return client
}

var legs = []domain.Leg{
	{ID: "base", Sender: "makerA::1220", Receiver: "alice::1220", Instrument: "CBTC", Amount: decimal.MustParse("0.5")},
	{ID: "quote", Sender: "alice::1220", Receiver: "makerA::1220", Instrument: "USDCx", Amount: decimal.MustParse("50000")},
}

func Test_Ledger_AllocateSendsTheSettlement(t *testing.T) {
	fake := &fakeLedger{}
	client := serve(t, fake)
	id := uuid.New()
	deadline := time.Date(2026, 10, 21, 12, 30, 0, 0, time.UTC)

	require.NoError(t, client.Allocate(t.Context(), id, deadline, legs, "alice::1220", "Buy 0.5 CBTC"))
	require.Equal(t, id.String(), fake.allocateIn.GetSettlementId())
	require.True(t, deadline.Equal(fake.allocateIn.GetDeadline().AsTime()))
	require.Equal(t, "alice::1220", fake.allocateIn.GetPartyId())
	require.Equal(t, "Buy 0.5 CBTC", fake.allocateIn.GetDescription())
	require.Len(t, fake.allocateIn.GetLegs(), 2)
	require.Equal(t, "quote", fake.allocateIn.GetLegs()[1].GetId())
	require.Equal(t, "alice::1220", fake.allocateIn.GetLegs()[1].GetSender())
	require.Equal(t, "makerA::1220", fake.allocateIn.GetLegs()[1].GetReceiver())
	require.Equal(t, "USDCx", fake.allocateIn.GetLegs()[1].GetInstrument())
	require.Equal(t, "50000.0", fake.allocateIn.GetLegs()[1].GetAmount())
}

func Test_Ledger_MapsErrors(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		is      error
		message string
	}{
		{
			"insufficient funds", status.Error(codes.FailedPrecondition, "insufficient funds: need 0.5 CBTC, held 0.1"),
			domain.ErrInsufficientFunds, "insufficient funds: need 0.5 CBTC, held 0.1",
		},
		{
			"bare insufficient funds", status.Error(codes.FailedPrecondition, "insufficient funds"),
			domain.ErrInsufficientFunds, "insufficient funds",
		},
		{
			"other precondition", status.Error(codes.FailedPrecondition, "settlement deadline passed"),
			domain.ErrUpstream, "",
		},
		{
			"invalid argument", status.Error(codes.InvalidArgument, "unknown instrument DOGE"),
			domain.ErrValidation, "validation failed: unknown instrument DOGE",
		},
		{"ledger down", status.Error(codes.Unavailable, "connection refused"), domain.ErrUpstream, ""},
		{"not found", status.Error(codes.NotFound, "unknown party"), domain.ErrUpstream, ""},
		{"internal", status.Error(codes.Internal, "canton said no"), domain.ErrUpstream, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := serve(t, &fakeLedger{allocateErr: c.err})

			err := client.Allocate(t.Context(), uuid.New(), time.Now(), legs, "alice::1220", "")
			require.ErrorIs(t, err, c.is)

			if c.message != "" {
				require.Equal(t, c.message, err.Error())
			}

			if !errors.Is(c.is, domain.ErrUpstream) {
				require.NotErrorIs(t, err, domain.ErrUpstream)
			}
		})
	}
}

func Test_Ledger_DeskAndSettlements(t *testing.T) {
	id := uuid.New()
	client := serve(t, &fakeLedger{settlements: &ledger_v1.GetSettlementsOutput{
		Settlements: []*ledger_v1.SettlementStatus{{
			SettlementId: id.String(),
			Parties: []*ledger_v1.PartyAllocation{
				{PartyId: "makerA::1220", Allocated: true},
				{PartyId: "alice::1220", Allocated: false},
			},
		}},
	}})

	desk, err := client.Desk(t.Context())
	require.NoError(t, err)
	require.Equal(t, domain.Desk{Operator: "operator::1220", Instruments: []string{"CBTC", "USDCx"}}, desk)

	allocated, err := client.Settlements(t.Context(), map[uuid.UUID][]domain.Leg{id: legs})
	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]map[string]bool{id: {"makerA::1220": true, "alice::1220": false}}, allocated)
}

func Test_Ledger_DownIsUpstream(t *testing.T) {
	otel.SilentModeInit()

	client, err := ledger.New(ledger.Config{Addr: "127.0.0.1:1"})
	require.NoError(t, err, "the client connects lazily")
	t.Cleanup(client.Close)

	_, err = client.Desk(t.Context())
	require.ErrorIs(t, err, domain.ErrUpstream)
}

func Test_Ledger_SettleOfConsumedAllocationsIsGone(t *testing.T) {
	client := serve(t, &fakeLedger{
		settleErr: status.Error(codes.NotFound, "request failed: not found: settlement has no allocation for makerA"),
	})

	_, err := client.Settle(t.Context(), uuid.New(), legs)
	require.ErrorIs(t, err, domain.ErrSettlementGone)
	require.NotErrorIs(t, err, domain.ErrUpstream)

	client = serve(t, &fakeLedger{})

	updateID, err := client.Settle(t.Context(), uuid.New(), legs)
	require.NoError(t, err)
	require.Equal(t, "update-1", updateID)

	client = serve(t, &fakeLedger{settleErr: status.Error(codes.Unavailable, "connection refused")})

	_, err = client.Settle(t.Context(), uuid.New(), legs)
	require.ErrorIs(t, err, domain.ErrUpstream)
}
