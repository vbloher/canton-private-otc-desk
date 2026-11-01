package desk

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/google/uuid"

	identityapi "github.com/vbloher/canton-private-otc-desk/maker-bot/internal/deskapi/identity"
)

func (c *Client) StartRegistration(
	ctx context.Context, displayName string, publicKey ed25519.PublicKey,
) (Registration, error) {
	resp, err := c.identity.RegisterStartWithResponse(ctx, identityapi.RegisterStartInput{
		DisplayName: displayName,
		PublicKey:   encode(publicKey),
	})
	if err != nil {
		return Registration{}, fmt.Errorf("register start: %w", err)
	}

	if resp.JSON200 == nil {
		return Registration{}, fmt.Errorf("register start: %w", statusError(resp.StatusCode(), resp.Body))
	}

	hash, err := decode("multiHash", resp.JSON200.MultiHash)
	if err != nil {
		return Registration{}, err
	}

	return Registration{ID: resp.JSON200.RegistrationID, PartyID: resp.JSON200.PartyID, MultiHash: hash}, nil
}

func (c *Client) RegisterParty(ctx context.Context, registrationID uuid.UUID, signature []byte) error {
	resp, err := c.identity.RegisterPartyWithResponse(ctx, identityapi.RegisterPartyInput{
		RegistrationID: registrationID,
		Signature:      encode(signature),
	})
	if err != nil {
		return fmt.Errorf("register party: %w", err)
	}

	if resp.JSON200 == nil {
		return fmt.Errorf("register party: %w", statusError(resp.StatusCode(), resp.Body))
	}

	return nil
}

func (c *Client) CompleteRegistration(ctx context.Context, registrationID uuid.UUID, signature []byte) (Me, error) {
	resp, err := c.identity.RegisterCompleteWithResponse(ctx, identityapi.RegisterCompleteInput{
		RegistrationID: registrationID,
		Signature:      encode(signature),
	})
	if err != nil {
		return Me{}, fmt.Errorf("register complete: %w", err)
	}

	if resp.JSON200 == nil {
		return Me{}, fmt.Errorf("register complete: %w", statusError(resp.StatusCode(), resp.Body))
	}

	return toMe(*resp.JSON200), nil
}

func (c *Client) SignIn(ctx context.Context) error {
	s, ok := c.currentSigner()
	if !ok {
		return ErrNoSigner
	}

	challenge, err := c.identity.CreateChallengeWithResponse(ctx, identityapi.ChallengeInput{PartyID: &s.partyID})
	if err != nil {
		return fmt.Errorf("challenge: %w", err)
	}

	if challenge.JSON200 == nil {
		return fmt.Errorf("challenge: %w", statusError(challenge.StatusCode(), challenge.Body))
	}

	text := challenge.JSON200.Challenge

	login, err := c.identity.LoginWithResponse(ctx, identityapi.LoginInput{
		Challenge: text,
		Signature: encode(s.sign([]byte(text))),
		PartyID:   &s.partyID,
	})
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	if login.JSON200 == nil {
		return fmt.Errorf("login: %w", statusError(login.StatusCode(), login.Body))
	}

	c.log.Info("signed in", "party", s.partyID)

	return nil
}

func (c *Client) SetMakerMode(ctx context.Context, enabled bool) error {
	resp, err := c.identity.SetMakerModeWithResponse(ctx, identityapi.MakerModeInput{Enabled: enabled})
	if err != nil {
		return fmt.Errorf("maker mode: %w", err)
	}

	if resp.JSON200 == nil {
		return fmt.Errorf("maker mode: %w", statusError(resp.StatusCode(), resp.Body))
	}

	return nil
}

func toMe(m identityapi.Me) Me {
	return Me{PartyID: m.PartyID, Hint: m.Hint, Kind: string(m.Kind), MakerMode: m.MakerMode}
}
