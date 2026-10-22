package domain

import (
	"encoding/json"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

// Disclosed is a contract handed to the ledger by value for a party that cannot see it.
type Disclosed struct {
	TemplateID       string
	ContractID       string
	CreatedEventBlob string
	SynchronizerID   string
}

// Command is one ledger command, ready for a submitter: Payload is the JSON API command.
type Command struct {
	Payload        json.RawMessage
	Disclosed      []Disclosed
	SynchronizerID string
}

// Prepared is an interactive submission waiting for the party's signature over Hash.
type Prepared struct {
	Transaction          string
	HashingSchemeVersion string
	Hash                 []byte
}

// Transaction is a committed submission: its update id and the contracts it created.
type Transaction struct {
	UpdateID string
	Created  []string
}

// AllocateSpec funds one slot of a settlement from Inputs.
type AllocateSpec struct {
	Operator     string
	SettlementID string
	Slot         Slot
	Deadline     time.Time
	RequestedAt  time.Time
	Inputs       []string
}

// SettleSpec settles every leg at once from the plan-matched allocations.
type SettleSpec struct {
	Operator       string
	SettlementID   string
	Legs           []TransferLeg
	AllocationCIDs []string
}

// TransferSpec offers Amount to Receiver, who has until ExecuteBefore to accept it.
type TransferSpec struct {
	Admin         string
	Sender        string
	Receiver      string
	InstrumentID  string
	Amount        decimal.Decimal
	Inputs        []string
	RequestedAt   time.Time
	ExecuteBefore time.Time
}

// OfferMintSpec offers newly minted tokens to Receiver (the sandbox faucet).
type OfferMintSpec struct {
	Admin        string
	Receiver     string
	InstrumentID string
	Amount       decimal.Decimal
	OfferedAt    time.Time
}
