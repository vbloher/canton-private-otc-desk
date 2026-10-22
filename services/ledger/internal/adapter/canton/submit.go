package canton

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

type disclosedContract struct {
	TemplateID       string `json:"templateId"`
	ContractID       string `json:"contractId"`
	CreatedEventBlob string `json:"createdEventBlob"`
	SynchronizerID   string `json:"synchronizerId,omitempty"`
}

func disclosedContracts(cmd domain.Command) []disclosedContract {
	out := make([]disclosedContract, 0, len(cmd.Disclosed))

	for _, d := range cmd.Disclosed {
		out = append(out, disclosedContract{
			TemplateID:       d.TemplateID,
			ContractID:       d.ContractID,
			CreatedEventBlob: d.CreatedEventBlob,
			SynchronizerID:   d.SynchronizerID,
		})
	}

	return out
}

type jsCommands struct {
	Commands           []json.RawMessage   `json:"commands"`
	CommandID          string              `json:"commandId"`
	UserID             string              `json:"userId"`
	ActAs              []string            `json:"actAs"`
	ReadAs             []string            `json:"readAs"`
	DisclosedContracts []disclosedContract `json:"disclosedContracts"`
	SynchronizerID     string              `json:"synchronizerId,omitempty"`
}

type transactionFormat struct {
	EventFormat      eventFormat `json:"eventFormat"`
	TransactionShape string      `json:"transactionShape"`
}

type submitRequest struct {
	Commands          jsCommands        `json:"commands"`
	TransactionFormat transactionFormat `json:"transactionFormat"`
}

type submitResponse struct {
	Transaction struct {
		UpdateID string `json:"updateId"`
		Events   []struct {
			CreatedEvent *struct {
				ContractID string `json:"contractId"`
			} `json:"CreatedEvent"`
		} `json:"events"`
	} `json:"transaction"`
}

// Submit runs a command of a party the participant hosts and waits for its transaction.
// A command id the ledger has already accepted answers ErrDuplicateCommand.
func (c *Canton) Submit(ctx context.Context, actAs, commandID string, cmd domain.Command) (domain.Transaction, error) {
	ctx, span := tracer.Start(ctx, "adapter canton Submit")
	defer span.End()

	if commandID == "" {
		commandID = uuid.NewString()
	}

	in := submitRequest{
		Commands: jsCommands{
			Commands:           []json.RawMessage{cmd.Payload},
			CommandID:          commandID,
			UserID:             c.userID,
			ActAs:              []string{actAs},
			ReadAs:             []string{},
			DisclosedContracts: disclosedContracts(cmd),
			SynchronizerID:     cmd.SynchronizerID,
		},
		TransactionFormat: transactionFormat{
			EventFormat:      wildcardFormat(actAs),
			TransactionShape: "TRANSACTION_SHAPE_ACS_DELTA",
		},
	}

	var out submitResponse

	err := c.call(ctx, http.MethodPost, "/v2/commands/submit-and-wait-for-transaction", in, &out, submitTimeout)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("submit-and-wait: %w", err)
	}

	tx := domain.Transaction{UpdateID: out.Transaction.UpdateID}

	for _, ev := range out.Transaction.Events {
		if ev.CreatedEvent != nil {
			tx.Created = append(tx.Created, ev.CreatedEvent.ContractID)
		}
	}

	return tx, nil
}

// Prepare builds an external party's transaction; the party signs the returned hash. The
// command id is the transaction's change id: the ledger executes one transaction per id, so
// a deterministic id keeps a retried request from landing twice.
func (c *Canton) Prepare(ctx context.Context, actAs, commandID string, cmd domain.Command) (domain.Prepared, error) {
	ctx, span := tracer.Start(ctx, "adapter canton Prepare")
	defer span.End()

	if commandID == "" {
		commandID = uuid.NewString()
	}

	in := struct {
		UserID                       string              `json:"userId"`
		CommandID                    string              `json:"commandId"`
		ActAs                        []string            `json:"actAs"`
		ReadAs                       []string            `json:"readAs"`
		Commands                     []json.RawMessage   `json:"commands"`
		VerboseHashing               bool                `json:"verboseHashing"`
		PackageIDSelectionPreference []string            `json:"packageIdSelectionPreference"`
		DisclosedContracts           []disclosedContract `json:"disclosedContracts"`
		SynchronizerID               string              `json:"synchronizerId,omitempty"`
	}{
		UserID:                       c.userID,
		CommandID:                    commandID,
		ActAs:                        []string{actAs},
		ReadAs:                       []string{},
		Commands:                     []json.RawMessage{cmd.Payload},
		PackageIDSelectionPreference: []string{},
		DisclosedContracts:           disclosedContracts(cmd),
		SynchronizerID:               cmd.SynchronizerID,
	}

	var out struct {
		PreparedTransaction     string `json:"preparedTransaction"`
		PreparedTransactionHash string `json:"preparedTransactionHash"`
		HashingSchemeVersion    string `json:"hashingSchemeVersion"`
	}

	err := c.call(ctx, http.MethodPost, "/v2/interactive-submission/prepare", in, &out, submitTimeout)
	if err != nil {
		return domain.Prepared{}, fmt.Errorf("prepare: %w", err)
	}

	hash, err := base64.StdEncoding.DecodeString(out.PreparedTransactionHash)
	if err != nil {
		return domain.Prepared{}, fmt.Errorf("%w: preparedTransactionHash: %w", domain.ErrLedger, err)
	}

	return domain.Prepared{
		Transaction:          out.PreparedTransaction,
		HashingSchemeVersion: out.HashingSchemeVersion,
		Hash:                 hash,
	}, nil
}

// Execute submits a prepared transaction with the party's signature over its hash and waits
// for its completion: once it returns, the transaction is committed (or rejected) and its
// effects are in the active contracts.
func (c *Canton) Execute(ctx context.Context, prepared domain.Prepared, party string, sig []byte) error {
	ctx, span := tracer.Start(ctx, "adapter canton Execute")
	defer span.End()

	type partySignature struct {
		Party      string      `json:"party"`
		Signatures []signature `json:"signatures"`
	}

	type partySignatures struct {
		Signatures []partySignature `json:"signatures"`
	}

	type deduplicationPeriod struct {
		Empty struct{} `json:"Empty"`
	}

	in := struct {
		UserID               string              `json:"userId"`
		SubmissionID         string              `json:"submissionId"`
		PreparedTransaction  string              `json:"preparedTransaction"`
		HashingSchemeVersion string              `json:"hashingSchemeVersion"`
		DeduplicationPeriod  deduplicationPeriod `json:"deduplicationPeriod"`
		PartySignatures      partySignatures     `json:"partySignatures"`
	}{
		UserID:               c.userID,
		SubmissionID:         uuid.NewString(),
		PreparedTransaction:  prepared.Transaction,
		HashingSchemeVersion: prepared.HashingSchemeVersion,
		PartySignatures: partySignatures{Signatures: []partySignature{{
			Party:      party,
			Signatures: []signature{ed25519Signature(sig, domain.Fingerprint(party))},
		}}},
	}

	var out struct {
		UpdateID string `json:"updateId"`
	}

	err := c.call(ctx, http.MethodPost, "/v2/interactive-submission/executeAndWait", in, &out, submitTimeout)
	if err != nil {
		return fmt.Errorf("executeAndWait: %w", err)
	}

	span.SetAttributes(attribute.String("update_id", out.UpdateID))

	return nil
}
