package bot

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/identity"
)

var ErrUnknownParty = errors.New("the desk does not know this party")

type enrollment struct {
	desk      Account
	path      string
	makerName string
}

func Enroll(ctx context.Context, d Account, path, makerName string) (identity.Identity, error) {
	e := enrollment{desk: d, path: path, makerName: makerName}

	id, err := identity.Load(path)

	switch {
	case errors.Is(err, os.ErrNotExist):
		id, err = identity.Generate()
		if err != nil {
			return identity.Identity{}, fmt.Errorf("identity.Generate: %w", err)
		}

		return e.start(ctx, id, false)
	case err != nil:
		return identity.Identity{}, fmt.Errorf("identity.Load: %w", err)
	case id.Complete:
		return e.signIn(ctx, id)
	default:
		return e.finish(ctx, id, false)
	}
}

func (e enrollment) start(ctx context.Context, id identity.Identity, restarted bool) (identity.Identity, error) {
	reg, err := e.desk.StartRegistration(ctx, e.makerName, id.PublicKey())
	if err != nil {
		return identity.Identity{}, fmt.Errorf("start registration: %w", err)
	}

	id.PartyID, id.Hint, id.RegistrationID, id.MultiHash, id.Complete = reg.PartyID, "", reg.ID, reg.MultiHash, false

	err = identity.Save(e.path, id)
	if err != nil {
		return identity.Identity{}, fmt.Errorf("identity.Save: %w", err)
	}

	return e.finish(ctx, id, restarted)
}

func (e enrollment) finish(ctx context.Context, id identity.Identity, restarted bool) (identity.Identity, error) {
	e.desk.SetSigner(id.PartyID, id.Sign)

	me, err := e.register(ctx, id)
	if errors.Is(err, desk.ErrGone) && !restarted {
		return e.start(ctx, id, true)
	}

	if err != nil {
		return identity.Identity{}, err
	}

	id.Hint, id.Complete = me.Hint, true

	err = identity.Save(e.path, id)
	if err != nil {
		return identity.Identity{}, fmt.Errorf("identity.Save: %w", err)
	}

	return id, nil
}

func (e enrollment) register(ctx context.Context, id identity.Identity) (desk.Me, error) {
	signature := id.Sign(id.MultiHash)

	err := e.desk.RegisterParty(ctx, id.RegistrationID, signature)
	if err != nil {
		return desk.Me{}, fmt.Errorf("register party: %w", err)
	}

	me, err := e.desk.CompleteRegistration(ctx, id.RegistrationID, signature)
	if err != nil {
		return desk.Me{}, fmt.Errorf("complete registration: %w", err)
	}

	return me, nil
}

func (e enrollment) signIn(ctx context.Context, id identity.Identity) (identity.Identity, error) {
	e.desk.SetSigner(id.PartyID, id.Sign)

	err := e.desk.SignIn(ctx)
	if errors.Is(err, desk.ErrNotFound) {
		return identity.Identity{}, fmt.Errorf(
			"%w: party %s: restore the identity file that belongs to it, or move %s away to register a new party",
			ErrUnknownParty, id.PartyID, e.path)
	}

	if err != nil {
		return identity.Identity{}, fmt.Errorf("sign in: %w", err)
	}

	return id, nil
}
