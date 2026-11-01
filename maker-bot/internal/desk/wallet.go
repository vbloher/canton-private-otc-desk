package desk

import (
	"context"
	"fmt"
	"math/big"
	"net/http"

	"github.com/google/uuid"

	walletapi "github.com/vbloher/canton-private-otc-desk/maker-bot/internal/deskapi/wallet"
)

func (c *Client) PendingActions(ctx context.Context) ([]Action, error) {
	resp, err := c.wallet.ListPendingActionsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("pending actions: %w", err)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("pending actions: %w", statusError(resp.StatusCode(), resp.Body))
	}

	actions := make([]Action, 0, len(*resp.JSON200))

	for _, a := range *resp.JSON200 {
		hash, err := decode("hash", a.Hash)
		if err != nil {
			return nil, err
		}

		actions = append(actions, Action{ID: a.ID, Purpose: string(a.Purpose), Description: a.Description, Hash: hash})
	}

	return actions, nil
}

func (c *Client) Execute(ctx context.Context, actionID uuid.UUID, signature []byte) error {
	resp, err := c.wallet.ExecuteActionWithResponse(ctx, actionID, walletapi.ExecuteInput{Signature: encode(signature)})
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	if resp.StatusCode() != http.StatusNoContent {
		return fmt.Errorf("execute: %w", statusError(resp.StatusCode(), resp.Body))
	}

	return nil
}

func (c *Client) Balances(ctx context.Context) (map[string]*big.Rat, error) {
	resp, err := c.wallet.GetBalancesWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("balances: %w", err)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("balances: %w", statusError(resp.StatusCode(), resp.Body))
	}

	balances := make(map[string]*big.Rat, len(*resp.JSON200))

	for _, b := range *resp.JSON200 {
		available, err := decimal("available", b.Available)
		if err != nil {
			return nil, err
		}

		balances[b.Symbol] = available
	}

	return balances, nil
}

func (c *Client) IncomingTransfers(ctx context.Context) ([]Transfer, error) {
	resp, err := c.wallet.ListIncomingWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("incoming transfers: %w", err)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("incoming transfers: %w", statusError(resp.StatusCode(), resp.Body))
	}

	transfers := make([]Transfer, 0, len(*resp.JSON200))

	for _, t := range *resp.JSON200 {
		amount, err := decimal("amount", t.Amount)
		if err != nil {
			return nil, err
		}

		transfers = append(transfers, Transfer{ContractID: t.ContractID, Sender: t.Sender, Symbol: t.Symbol, Amount: amount})
	}

	return transfers, nil
}

func (c *Client) AcceptTransfer(ctx context.Context, contractID string) error {
	resp, err := c.wallet.AcceptIncomingWithResponse(ctx, contractID)
	if err != nil {
		return fmt.Errorf("accept transfer: %w", err)
	}

	if resp.JSON202 == nil {
		return fmt.Errorf("accept transfer: %w", statusError(resp.StatusCode(), resp.Body))
	}

	return nil
}

func (c *Client) Network(ctx context.Context) (Network, error) {
	resp, err := c.wallet.GetNetworkWithResponse(ctx)
	if err != nil {
		return Network{}, fmt.Errorf("network: %w", err)
	}

	if resp.JSON200 == nil {
		return Network{}, fmt.Errorf("network: %w", statusError(resp.StatusCode(), resp.Body))
	}

	network := Network{Name: string(resp.JSON200.Name), Faucet: make([]Drip, 0, len(resp.JSON200.Faucet))}

	for _, d := range resp.JSON200.Faucet {
		amount, err := decimal("amount", d.Amount)
		if err != nil {
			return Network{}, err
		}

		network.Faucet = append(network.Faucet, Drip{Symbol: d.Symbol, Amount: amount})
	}

	return network, nil
}

func (c *Client) Faucet(ctx context.Context) error {
	resp, err := c.wallet.FaucetWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("faucet: %w", err)
	}

	if resp.JSON202 == nil {
		return fmt.Errorf("faucet: %w", statusError(resp.StatusCode(), resp.Body))
	}

	return nil
}
