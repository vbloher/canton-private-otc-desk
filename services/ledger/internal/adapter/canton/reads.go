package canton

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

type instrumentID struct {
	Admin string `json:"admin"`
	ID    string `json:"id"`
}

// Holdings are the token-standard holdings the party sees, read through the Holding v1
// interface; seeing a holding does not make it the party's.
func (c *Canton) Holdings(ctx context.Context, party string) ([]domain.Holding, error) {
	ctx, span := tracer.Start(ctx, "adapter canton Holdings")
	defer span.End()

	contracts, err := c.activeContracts(ctx, interfaceFormat(party, holdingV1))
	if err != nil {
		return nil, fmt.Errorf("c.activeContracts: %w", err)
	}

	return holdingsOf(contracts)
}

func holdingsOf(contracts []activeContract) ([]domain.Holding, error) {
	var holdings []domain.Holding

	for _, contract := range contracts {
		for _, raw := range contract.CreatedEvent.views() {
			var view struct {
				Owner        string       `json:"owner"`
				InstrumentID instrumentID `json:"instrumentId"`
				Amount       string       `json:"amount"`
				Lock         *struct {
					// Only expiresAt makes a lock lapse: TestTokenV2 ignores expiresAfter.
					ExpiresAt *time.Time `json:"expiresAt"`
				} `json:"lock"`
			}

			err := json.Unmarshal(raw, &view)
			if err != nil {
				return nil, fmt.Errorf("%w: holding view: %w", domain.ErrLedger, err)
			}

			amount, err := decimal.Parse(view.Amount)
			if err != nil {
				return nil, fmt.Errorf("%w: holding %s: %w", domain.ErrLedger, contract.CreatedEvent.ContractID, err)
			}

			holding := domain.Holding{
				ContractID:   contract.CreatedEvent.ContractID,
				Owner:        view.Owner,
				Admin:        view.InstrumentID.Admin,
				InstrumentID: view.InstrumentID.ID,
				Amount:       amount,
				Locked:       view.Lock != nil,
			}

			if view.Lock != nil && view.Lock.ExpiresAt != nil {
				holding.LockedUntil = view.Lock.ExpiresAt.UTC()
			}

			holdings = append(holdings, holding)
		}
	}

	return holdings, nil
}

// Allocations are the v2 allocations the executor sees: every allocation of its settlements.
func (c *Canton) Allocations(ctx context.Context, executor string) ([]domain.Allocation, error) {
	ctx, span := tracer.Start(ctx, "adapter canton Allocations")
	defer span.End()

	contracts, err := c.activeContracts(ctx, interfaceFormat(executor, allocationV2))
	if err != nil {
		return nil, fmt.Errorf("c.activeContracts: %w", err)
	}

	return allocationsOf(contracts)
}

func allocationsOf(contracts []activeContract) ([]domain.Allocation, error) {
	var allocations []domain.Allocation

	for _, contract := range contracts {
		for _, raw := range contract.CreatedEvent.views() {
			var view struct {
				Settlement struct {
					ID string `json:"id"`
				} `json:"settlement"`
				Allocation struct {
					Admin      string `json:"admin"`
					Authorizer struct {
						Owner string `json:"owner"`
					} `json:"authorizer"`
					TransferLegSides []struct {
						InstrumentID string `json:"instrumentId"`
					} `json:"transferLegSides"`
				} `json:"allocation"`
			}

			err := json.Unmarshal(raw, &view)
			if err != nil {
				return nil, fmt.Errorf("%w: allocation view: %w", domain.ErrLedger, err)
			}

			var instruments []string

			for _, side := range view.Allocation.TransferLegSides {
				if !slices.Contains(instruments, side.InstrumentID) {
					instruments = append(instruments, side.InstrumentID)
				}
			}

			allocations = append(allocations, domain.Allocation{
				ContractID:    contract.CreatedEvent.ContractID,
				SettlementID:  view.Settlement.ID,
				Authorizer:    view.Allocation.Authorizer.Owner,
				Admin:         view.Allocation.Admin,
				InstrumentIDs: instruments,
			})
		}
	}

	return allocations, nil
}

// TransferInstructions are the pending transfers addressed to the receiver. The receiver
// also sees the ones it sends, so the view's receiver decides, never the template.
func (c *Canton) TransferInstructions(ctx context.Context, receiver string) ([]domain.IncomingTransfer, error) {
	ctx, span := tracer.Start(ctx, "adapter canton TransferInstructions")
	defer span.End()

	contracts, err := c.activeContracts(ctx, interfaceFormat(receiver, transferInstructionV1))
	if err != nil {
		return nil, fmt.Errorf("c.activeContracts: %w", err)
	}

	return transfersOf(contracts, receiver)
}

// noOwnerHint is how CIP-112 renders, in a v1 view, an account without an owner: the mint
// account a faucet offer comes from. It is no party to show or to notify.
const noOwnerHint = "cip-112_no-owner"

func senderOf(party string) string {
	if domain.Hint(party) == noOwnerHint {
		return ""
	}

	return party
}

func transfersOf(contracts []activeContract, receiver string) ([]domain.IncomingTransfer, error) {
	var transfers []domain.IncomingTransfer

	for _, contract := range contracts {
		for _, raw := range contract.CreatedEvent.views() {
			var view struct {
				Transfer struct {
					Sender           string       `json:"sender"`
					Receiver         string       `json:"receiver"`
					Amount           string       `json:"amount"`
					InstrumentID     instrumentID `json:"instrumentId"`
					InputHoldingCids []string     `json:"inputHoldingCids"`
				} `json:"transfer"`
			}

			err := json.Unmarshal(raw, &view)
			if err != nil {
				return nil, fmt.Errorf("%w: transfer instruction view: %w", domain.ErrLedger, err)
			}

			if view.Transfer.Receiver != receiver {
				continue
			}

			amount, err := decimal.Parse(view.Transfer.Amount)
			if err != nil {
				return nil, fmt.Errorf("%w: transfer %s: %w", domain.ErrLedger, contract.CreatedEvent.ContractID, err)
			}

			transfers = append(transfers, domain.IncomingTransfer{
				ContractID:   contract.CreatedEvent.ContractID,
				Sender:       senderOf(view.Transfer.Sender),
				Admin:        view.Transfer.InstrumentID.Admin,
				InstrumentID: view.Transfer.InstrumentID.ID,
				Amount:       amount,
				Inputs:       view.Transfer.InputHoldingCids,
			})
		}
	}

	return transfers, nil
}

// TokenRules is the sandbox registry stand-in: the factory for allocate, settle and
// transfer, disclosed to every party that exercises it.
func (c *Canton) TokenRules(ctx context.Context, admin string) (domain.Disclosed, error) {
	ctx, span := tracer.Start(ctx, "adapter canton TokenRules")
	defer span.End()

	contracts, err := c.activeContracts(ctx, templateFormat(admin, tokenRulesTemplate))
	if err != nil {
		return domain.Disclosed{}, fmt.Errorf("c.activeContracts: %w", err)
	}

	disclosed := disclosedOf(contracts)
	if len(disclosed) == 0 {
		return domain.Disclosed{}, fmt.Errorf("%w: no TokenRules for %s: is the sandbox seeded?", domain.ErrLedger, admin)
	}

	return disclosed[0], nil
}

// Tokens are every holding contract of the admin's instruments, disclosed for settle and
// cancel: the executor is no stakeholder of the locked holdings they touch.
func (c *Canton) Tokens(ctx context.Context, admin string) ([]domain.Disclosed, error) {
	ctx, span := tracer.Start(ctx, "adapter canton Tokens")
	defer span.End()

	contracts, err := c.activeContracts(ctx, templateFormat(admin, tokenTemplate))
	if err != nil {
		return nil, fmt.Errorf("c.activeContracts: %w", err)
	}

	return disclosedOf(contracts), nil
}

func disclosedOf(contracts []activeContract) []domain.Disclosed {
	disclosed := make([]domain.Disclosed, 0, len(contracts))

	for _, contract := range contracts {
		if contract.CreatedEvent.CreatedEventBlob == "" {
			continue
		}

		disclosed = append(disclosed, domain.Disclosed{
			TemplateID:       contract.CreatedEvent.TemplateID,
			ContractID:       contract.CreatedEvent.ContractID,
			CreatedEventBlob: contract.CreatedEvent.CreatedEventBlob,
			SynchronizerID:   contract.SynchronizerID,
		})
	}

	return disclosed
}
