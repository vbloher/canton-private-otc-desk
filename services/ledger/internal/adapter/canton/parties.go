package canton

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const maxPartyPages = 1000

type signature struct {
	Format               string `json:"format"`
	Signature            string `json:"signature"`
	SignedBy             string `json:"signedBy"`
	SigningAlgorithmSpec string `json:"signingAlgorithmSpec"`
}

func ed25519Signature(sig []byte, fingerprint string) signature {
	return signature{
		Format:               "SIGNATURE_FORMAT_RAW",
		Signature:            base64.StdEncoding.EncodeToString(sig),
		SignedBy:             fingerprint,
		SigningAlgorithmSpec: "SIGNING_ALGORITHM_SPEC_ED25519",
	}
}

func (c *Canton) Ready(ctx context.Context) bool {
	ctx, span := tracer.Start(ctx, "adapter canton Ready")
	defer span.End()

	return c.call(ctx, http.MethodGet, "/readyz", nil, nil, readyTimeout) == nil
}

func (c *Canton) ResolveParty(ctx context.Context, hint string) (string, error) {
	ctx, span := tracer.Start(ctx, "adapter canton ResolveParty")
	defer span.End()

	ids, err := c.partiesByHint(ctx, hint)
	if err != nil {
		return "", err
	}

	switch len(ids) {
	case 0:
		return "", fmt.Errorf("%w: party %q", domain.ErrNotFound, hint)
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("%w: %q names %s: is the ledger seeded twice?", domain.ErrAmbiguousParty, hint,
			strings.Join(ids, ", "))
	}
}

type partiesPage struct {
	PartyDetails []struct {
		Party string `json:"party"`
	} `json:"partyDetails"`
	NextPageToken string `json:"nextPageToken"`
}

func (c *Canton) partiesByHint(ctx context.Context, hint string) ([]string, error) {
	var (
		ids   []string
		token string
	)

	for range maxPartyPages {
		var page partiesPage

		path := "/v2/parties?filter-party=" + url.QueryEscape(hint) + "&pageToken=" + url.QueryEscape(token)

		err := c.call(ctx, http.MethodGet, path, nil, &page, readTimeout)
		if err != nil {
			return nil, fmt.Errorf("parties: %w", err)
		}

		for _, p := range page.PartyDetails {
			if p.Party == hint || strings.HasPrefix(p.Party, hint+"::") {
				ids = append(ids, p.Party)
			}
		}

		if page.NextPageToken == "" || page.NextPageToken == token {
			break
		}

		token = page.NextPageToken
	}

	return ids, nil
}

func (c *Canton) SynchronizerID(ctx context.Context, party string) (string, error) {
	ctx, span := tracer.Start(ctx, "adapter canton SynchronizerID")
	defer span.End()

	var out struct {
		ConnectedSynchronizers []struct {
			SynchronizerID string `json:"synchronizerId"`
		} `json:"connectedSynchronizers"`
	}

	err := c.call(ctx, http.MethodGet, "/v2/state/connected-synchronizers?party="+url.QueryEscape(party),
		nil, &out, readTimeout)
	if err != nil {
		return "", fmt.Errorf("connected-synchronizers: %w", err)
	}

	if len(out.ConnectedSynchronizers) == 0 {
		return "", fmt.Errorf("%w: %s is connected to no synchronizer", domain.ErrLedger, party)
	}

	return out.ConnectedSynchronizers[0].SynchronizerID, nil
}

func (c *Canton) GenerateExternalParty(ctx context.Context, synchronizerID, hint string, publicKey []byte,
) (domain.Topology, error) {
	ctx, span := tracer.Start(ctx, "adapter canton GenerateExternalParty")
	defer span.End()

	type publicKeyJSON struct {
		Format  string `json:"format"`
		KeyData string `json:"keyData"`
		KeySpec string `json:"keySpec"`
	}

	in := struct {
		Synchronizer string        `json:"synchronizer"`
		PartyHint    string        `json:"partyHint"`
		PublicKey    publicKeyJSON `json:"publicKey"`
	}{
		Synchronizer: synchronizerID,
		PartyHint:    hint,
		PublicKey: publicKeyJSON{
			Format:  "CRYPTO_KEY_FORMAT_RAW",
			KeyData: base64.StdEncoding.EncodeToString(publicKey),
			KeySpec: "SIGNING_KEY_SPEC_EC_CURVE25519",
		},
	}

	var out struct {
		PartyID              string   `json:"partyId"`
		PublicKeyFingerprint string   `json:"publicKeyFingerprint"`
		TopologyTransactions []string `json:"topologyTransactions"`
		MultiHash            string   `json:"multiHash"`
	}

	err := c.call(ctx, http.MethodPost, "/v2/parties/external/generate-topology", in, &out, readTimeout)
	if err != nil {
		return domain.Topology{}, fmt.Errorf("generate-topology: %w", err)
	}

	multiHash, err := base64.StdEncoding.DecodeString(out.MultiHash)
	if err != nil {
		return domain.Topology{}, fmt.Errorf("%w: multiHash: %w", domain.ErrLedger, err)
	}

	return domain.Topology{
		PartyID:      out.PartyID,
		Fingerprint:  out.PublicKeyFingerprint,
		MultiHash:    multiHash,
		Transactions: out.TopologyTransactions,
	}, nil
}

func (c *Canton) AllocateExternalParty(ctx context.Context, synchronizerID string, transactions []string,
	sig []byte, fingerprint string,
) (string, error) {
	ctx, span := tracer.Start(ctx, "adapter canton AllocateExternalParty")
	defer span.End()

	user, err := c.user()
	if err != nil {
		return "", err
	}

	type signedTransaction struct {
		Transaction string      `json:"transaction"`
		Signatures  []signature `json:"signatures"`
	}

	onboarding := make([]signedTransaction, 0, len(transactions))
	for _, tx := range transactions {
		onboarding = append(onboarding, signedTransaction{Transaction: tx, Signatures: []signature{}})
	}

	in := struct {
		Synchronizer           string              `json:"synchronizer"`
		OnboardingTransactions []signedTransaction `json:"onboardingTransactions"`
		MultiHashSignatures    []signature         `json:"multiHashSignatures"`
		UserID                 string              `json:"userId"`
		WaitForAllocation      bool                `json:"waitForAllocation"`
	}{
		Synchronizer:           synchronizerID,
		OnboardingTransactions: onboarding,
		MultiHashSignatures:    []signature{ed25519Signature(sig, fingerprint)},
		UserID:                 user,
		WaitForAllocation:      true,
	}

	var out struct {
		PartyID string `json:"partyId"`
	}

	err = c.call(ctx, http.MethodPost, "/v2/parties/external/allocate", in, &out, submitTimeout)
	if err != nil {
		return "", fmt.Errorf("external/allocate: %w", err)
	}

	return out.PartyID, nil
}
