package ledger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

type fakeClient struct {
	pb.LedgerV1Client

	resolve  func(in *pb.ResolvePartyInput) (*pb.ResolvePartyOutput, error)
	generate func(in *pb.GenerateExternalPartyInput) (*pb.GenerateExternalPartyOutput, error)
	allocate func(in *pb.AllocateExternalPartyInput) (*pb.AllocateExternalPartyOutput, error)
}

func (f fakeClient) ResolveParty(
	_ context.Context, in *pb.ResolvePartyInput, _ ...grpc.CallOption,
) (*pb.ResolvePartyOutput, error) {
	return f.resolve(in)
}

func (f fakeClient) GenerateExternalParty(
	_ context.Context, in *pb.GenerateExternalPartyInput, _ ...grpc.CallOption,
) (*pb.GenerateExternalPartyOutput, error) {
	return f.generate(in)
}

func (f fakeClient) AllocateExternalParty(
	_ context.Context, in *pb.AllocateExternalPartyInput, _ ...grpc.CallOption,
) (*pb.AllocateExternalPartyOutput, error) {
	return f.allocate(in)
}

func newLedger(t *testing.T, client fakeClient) *Ledger {
	t.Helper()

	otel.SilentModeInit()

	return &Ledger{client: client}
}

func Test_ResolveParty(t *testing.T) {
	l := newLedger(t, fakeClient{resolve: func(in *pb.ResolvePartyInput) (*pb.ResolvePartyOutput, error) {
		require.Equal(t, "makerA", in.GetHint())

		return &pb.ResolvePartyOutput{PartyId: "makerA::1220ab"}, nil
	}})

	partyID, err := l.ResolveParty(t.Context(), "makerA")
	require.NoError(t, err)
	require.Equal(t, "makerA::1220ab", partyID)
}

func Test_ResolveParty_NotFound(t *testing.T) {
	l := newLedger(t, fakeClient{resolve: func(*pb.ResolvePartyInput) (*pb.ResolvePartyOutput, error) {
		return nil, status.Error(codes.NotFound, "no such party")
	}})

	_, err := l.ResolveParty(t.Context(), "ghost")
	require.ErrorIs(t, err, domain.ErrNotFound)
	require.NotErrorIs(t, err, domain.ErrLedger)
}

func Test_ResolveParty_OtherFailuresAreLedgerFailures(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.Internal, codes.DeadlineExceeded, codes.InvalidArgument} {
		l := newLedger(t, fakeClient{resolve: func(*pb.ResolvePartyInput) (*pb.ResolvePartyOutput, error) {
			return nil, status.Error(code, "boom")
		}})

		_, err := l.ResolveParty(t.Context(), "makerA")
		require.ErrorIs(t, err, domain.ErrLedger, code.String())
		require.NotErrorIs(t, err, domain.ErrNotFound, code.String())
	}
}

func Test_ResolveParty_EmptyAnswer(t *testing.T) {
	l := newLedger(t, fakeClient{resolve: func(*pb.ResolvePartyInput) (*pb.ResolvePartyOutput, error) {
		return &pb.ResolvePartyOutput{}, nil
	}})

	_, err := l.ResolveParty(t.Context(), "makerA")
	require.ErrorIs(t, err, domain.ErrLedger)
}

func Test_GenerateExternalParty(t *testing.T) {
	publicKey := []byte("0123456789abcdef0123456789abcdef")

	l := newLedger(t, fakeClient{generate: func(in *pb.GenerateExternalPartyInput) (*pb.GenerateExternalPartyOutput, error) {
		require.Equal(t, "alice-3f9a1c", in.GetHint())
		require.Equal(t, publicKey, in.GetPublicKey())

		return &pb.GenerateExternalPartyOutput{
			PartyId:              "alice-3f9a1c::1220ab",
			Fingerprint:          "1220ab",
			MultiHash:            []byte("multi-hash"),
			TopologyTransactions: []string{"tx-1", "tx-2"},
		}, nil
	}})

	topology, err := l.GenerateExternalParty(t.Context(), "alice-3f9a1c", publicKey)
	require.NoError(t, err)
	require.Equal(t, domain.Topology{
		PartyID:      "alice-3f9a1c::1220ab",
		Fingerprint:  "1220ab",
		MultiHash:    []byte("multi-hash"),
		Transactions: []string{"tx-1", "tx-2"},
	}, topology)
}

func Test_GenerateExternalParty_Failure(t *testing.T) {
	l := newLedger(t, fakeClient{generate: func(*pb.GenerateExternalPartyInput) (*pb.GenerateExternalPartyOutput, error) {
		return nil, status.Error(codes.Unavailable, "ledger is down")
	}})

	_, err := l.GenerateExternalParty(t.Context(), "alice-3f9a1c", nil)
	require.ErrorIs(t, err, domain.ErrLedger)
}

func Test_GenerateExternalParty_IncompleteTopology(t *testing.T) {
	complete := &pb.GenerateExternalPartyOutput{
		PartyId:              "alice-3f9a1c::1220ab",
		Fingerprint:          "1220ab",
		MultiHash:            []byte("multi-hash"),
		TopologyTransactions: []string{"tx-1"},
	}

	for name, mutate := range map[string]func(o *pb.GenerateExternalPartyOutput){
		"no party":        func(o *pb.GenerateExternalPartyOutput) { o.PartyId = "" },
		"no fingerprint":  func(o *pb.GenerateExternalPartyOutput) { o.Fingerprint = "" },
		"no multi hash":   func(o *pb.GenerateExternalPartyOutput) { o.MultiHash = nil },
		"no transactions": func(o *pb.GenerateExternalPartyOutput) { o.TopologyTransactions = nil },
	} {
		output := &pb.GenerateExternalPartyOutput{
			PartyId:              complete.GetPartyId(),
			Fingerprint:          complete.GetFingerprint(),
			MultiHash:            complete.GetMultiHash(),
			TopologyTransactions: complete.GetTopologyTransactions(),
		}
		mutate(output)

		l := newLedger(t, fakeClient{generate: func(*pb.GenerateExternalPartyInput) (*pb.GenerateExternalPartyOutput, error) {
			return output, nil
		}})

		_, err := l.GenerateExternalParty(t.Context(), "alice-3f9a1c", nil)
		require.ErrorIs(t, err, domain.ErrLedger, name)
	}
}

func Test_AllocateExternalParty(t *testing.T) {
	topology := domain.Topology{
		PartyID:      "alice-3f9a1c::1220ab",
		Fingerprint:  "1220ab",
		MultiHash:    []byte("multi-hash"),
		Transactions: []string{"tx-1", "tx-2"},
	}

	l := newLedger(t, fakeClient{allocate: func(in *pb.AllocateExternalPartyInput) (*pb.AllocateExternalPartyOutput, error) {
		require.Equal(t, []string{"tx-1", "tx-2"}, in.GetTopologyTransactions())
		require.Equal(t, []byte("signature"), in.GetSignature())
		require.Equal(t, "1220ab", in.GetFingerprint())

		return &pb.AllocateExternalPartyOutput{PartyId: "alice-3f9a1c::1220ab"}, nil
	}})

	partyID, err := l.AllocateExternalParty(t.Context(), topology, []byte("signature"))
	require.NoError(t, err)
	require.Equal(t, "alice-3f9a1c::1220ab", partyID)
}

func Test_AllocateExternalParty_Failure(t *testing.T) {
	l := newLedger(t, fakeClient{allocate: func(*pb.AllocateExternalPartyInput) (*pb.AllocateExternalPartyOutput, error) {
		return nil, status.Error(codes.Internal, "canton said no")
	}})

	_, err := l.AllocateExternalParty(t.Context(), domain.Topology{}, nil)
	require.ErrorIs(t, err, domain.ErrLedger)
}

func Test_AllocateExternalParty_TopologyFrozen(t *testing.T) {
	l := newLedger(t, fakeClient{allocate: func(*pb.AllocateExternalPartyInput) (*pb.AllocateExternalPartyOutput, error) {
		return nil, status.Error(codes.FailedPrecondition, "the network is being upgraded")
	}})

	_, err := l.AllocateExternalParty(t.Context(), domain.Topology{}, nil)
	require.ErrorIs(t, err, domain.ErrOnboardingPaused)
	require.NotErrorIs(t, err, domain.ErrLedger)
}

func Test_AllocateExternalParty_EmptyAnswer(t *testing.T) {
	l := newLedger(t, fakeClient{allocate: func(*pb.AllocateExternalPartyInput) (*pb.AllocateExternalPartyOutput, error) {
		return &pb.AllocateExternalPartyOutput{}, nil
	}})

	_, err := l.AllocateExternalParty(t.Context(), domain.Topology{}, nil)
	require.ErrorIs(t, err, domain.ErrLedger)
}
