package identity_test

import (
	"context"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/adapter/identity"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

type server struct {
	pb.UnimplementedIdentityV1Server

	user *pb.User
	err  error

	sessionToken string
	apiKey       string
}

func (s *server) Authenticate(_ context.Context, in *pb.AuthenticateInput) (*pb.User, error) {
	s.sessionToken = in.GetSessionToken()

	return s.user, s.err
}

func (s *server) AuthenticateAPIKey(_ context.Context, in *pb.AuthenticateAPIKeyInput) (*pb.User, error) {
	s.apiKey = in.GetApiKey()

	return s.user, s.err
}

func dial(t *testing.T, srv pb.IdentityV1Server) *identity.Identity {
	t.Helper()

	otel.SilentModeInit()

	listener := bufconn.Listen(1 << 20)

	grpcServer := grpc.NewServer()
	pb.RegisterIdentityV1Server(grpcServer, srv)

	go func() { _ = grpcServer.Serve(listener) }()

	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	require.NoError(t, err)

	t.Cleanup(func() { _ = conn.Close() })

	return identity.New(conn)
}

type call struct {
	name string
	do   func(*identity.Identity) (principal.Principal, error)
	sent func(*server) string
}

func calls() []call {
	return []call{
		{
			name: "Authenticate",
			do: func(i *identity.Identity) (principal.Principal, error) {
				return i.Authenticate(context.Background(), "the-credential")
			},
			sent: func(s *server) string { return s.sessionToken },
		},
		{
			name: "AuthenticateAPIKey",
			do: func(i *identity.Identity) (principal.Principal, error) {
				return i.AuthenticateAPIKey(context.Background(), "the-credential")
			},
			sent: func(s *server) string { return s.apiKey },
		},
	}
}

func Test_Identity_ReturnsPrincipal(t *testing.T) {
	userID := uuid.New()

	for _, c := range calls() {
		t.Run(c.name, func(t *testing.T) {
			srv := &server{user: &pb.User{
				Id:          userID.String(),
				PartyId:     "alice-3f9a1c::1220ab",
				Hint:        "alice-3f9a1c",
				DisplayName: "Alice",
				MakerMode:   true,
			}}

			actual, err := c.do(dial(t, srv))
			require.NoError(t, err)
			require.Equal(t, "the-credential", c.sent(srv))
			require.Equal(t, principal.Principal{UserID: userID, PartyID: "alice-3f9a1c::1220ab", Hint: "alice-3f9a1c"}, actual)
		})
	}
}

func Test_Identity_UnknownCredentialIsUnauthorized(t *testing.T) {
	for _, code := range []codes.Code{codes.Unauthenticated, codes.NotFound} {
		for _, c := range calls() {
			t.Run(c.name+"/"+code.String(), func(t *testing.T) {
				actual, err := c.do(dial(t, &server{err: status.Error(code, "no such credential")}))
				require.ErrorIs(t, err, domain.ErrUnauthorized)
				require.Equal(t, principal.Principal{}, actual)
			})
		}
	}
}

func Test_Identity_OtherFailuresAreNotUnauthorized(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.Internal, codes.DeadlineExceeded, codes.PermissionDenied} {
		for _, c := range calls() {
			t.Run(c.name+"/"+code.String(), func(t *testing.T) {
				_, err := c.do(dial(t, &server{err: status.Error(code, "boom")}))
				require.Error(t, err)
				require.NotErrorIs(t, err, domain.ErrUnauthorized)
				require.Equal(t, code, status.Code(err), "the gRPC status stays reachable for logging")
			})
		}
	}
}

func Test_Identity_RejectsUnusableUser(t *testing.T) {
	valid := pb.User{Id: uuid.NewString(), PartyId: "alice::1220ab", Hint: "alice"}

	tests := map[string]func(u *pb.User){
		"id is not a uuid": func(u *pb.User) { u.Id = "42" },
		"no party id":      func(u *pb.User) { u.PartyId = "" },
		"no hint":          func(u *pb.User) { u.Hint = "" },
	}

	for name, damage := range tests {
		for _, c := range calls() {
			t.Run(c.name+"/"+name, func(t *testing.T) {
				user := &pb.User{Id: valid.GetId(), PartyId: valid.GetPartyId(), Hint: valid.GetHint()}
				damage(user)

				actual, err := c.do(dial(t, &server{user: user}))
				require.ErrorIs(t, err, domain.ErrInvalidUser)
				require.NotErrorIs(t, err, domain.ErrUnauthorized)
				require.Equal(t, principal.Principal{}, actual)
			})
		}
	}
}
