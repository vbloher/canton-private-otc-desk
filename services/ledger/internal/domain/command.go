package domain

import (
	"encoding/json"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

type Disclosed struct {
	TemplateID       string
	ContractID       string
	CreatedEventBlob string
	SynchronizerID   string
}

type Command struct {
	Commands       []json.RawMessage
	Disclosed      []Disclosed
	SynchronizerID string
}

type Prepared struct {
	Transaction          string
	HashingSchemeVersion string
	Hash                 []byte
}

type Transaction struct {
	UpdateID string
	Created  []string
}

type AllocateSpec struct {
	Operator     string
	SettlementID string
	Slot         Slot
	Deadline     time.Time
	RequestedAt  time.Time
	Inputs       []string
}

type SettleSpec struct {
	Operator     string
	SettlementID string
	Batches      []Batch
}

type TransferSpec struct {
	Registry      string
	Admin         string
	Sender        string
	Receiver      string
	InstrumentID  string
	Amount        decimal.Decimal
	Inputs        []string
	RequestedAt   time.Time
	ExecuteBefore time.Time
}
