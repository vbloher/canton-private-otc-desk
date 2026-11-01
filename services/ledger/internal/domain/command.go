package domain

import (
	"encoding/json"
	"slices"
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

func Combine(cmds []Command) (Command, bool) {
	var combined Command

	for _, cmd := range cmds {
		if cmd.SynchronizerID != "" {
			if combined.SynchronizerID != "" && combined.SynchronizerID != cmd.SynchronizerID {
				return Command{}, false
			}

			combined.SynchronizerID = cmd.SynchronizerID
		}

		combined.Commands = append(combined.Commands, cmd.Commands...)

		for _, d := range cmd.Disclosed {
			if !slices.ContainsFunc(combined.Disclosed, func(x Disclosed) bool { return x.ContractID == d.ContractID }) {
				combined.Disclosed = append(combined.Disclosed, d)
			}
		}
	}

	return combined, true
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
