package canton

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

// The JSON shapes below are the token standard's choice arguments as the JSON Ledger API
// encodes them: Decimal as a string, Optional as the value or null, Time as RFC 3339.

type exerciseCommand struct {
	ExerciseCommand exercise `json:"ExerciseCommand"`
}

type exercise struct {
	TemplateID     string `json:"templateId"`
	ContractID     string `json:"contractId"`
	Choice         string `json:"choice"`
	ChoiceArgument any    `json:"choiceArgument"`
}

type metadata struct {
	Values map[string]string `json:"values"`
}

// account is a basic token-standard account: the party alone, no provider.
type account struct {
	Owner    string  `json:"owner"`
	Provider *string `json:"provider"`
	ID       string  `json:"id"`
}

type extraArgs struct {
	Context choiceContext `json:"context"`
	Meta    metadata      `json:"meta"`
}

type choiceContext struct {
	Values map[string]anyValue `json:"values"`
}

type anyValue struct {
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

type settlementInfo struct {
	Executors []string `json:"executors"`
	ID        string   `json:"id"`
	Cid       *string  `json:"cid"`
	Meta      metadata `json:"meta"`
}

type transferLegSide struct {
	TransferLegID string   `json:"transferLegId"`
	Side          string   `json:"side"`
	Otherside     account  `json:"otherside"`
	Amount        string   `json:"amount"`
	InstrumentID  string   `json:"instrumentId"`
	Meta          metadata `json:"meta"`
}

type allocationSpec struct {
	Admin                string            `json:"admin"`
	Authorizer           account           `json:"authorizer"`
	TransferLegSides     []transferLegSide `json:"transferLegSides"`
	SettlementDeadline   string            `json:"settlementDeadline"`
	NextIterationFunding *struct{}         `json:"nextIterationFunding"`
	Committed            bool              `json:"committed"`
	Meta                 metadata          `json:"meta"`
}

type allocateArgs struct {
	Settlement       settlementInfo `json:"settlement"`
	Allocation       allocationSpec `json:"allocation"`
	RequestedAt      string         `json:"requestedAt"`
	InputHoldingCids []string       `json:"inputHoldingCids"`
	ExtraArgs        extraArgs      `json:"extraArgs"`
	Actors           []string       `json:"actors"`
}

type transferLeg struct {
	TransferLegID string   `json:"transferLegId"`
	Sender        account  `json:"sender"`
	Receiver      account  `json:"receiver"`
	Amount        string   `json:"amount"`
	InstrumentID  string   `json:"instrumentId"`
	Meta          metadata `json:"meta"`
}

type batchAllocation struct {
	AllocationCid         string     `json:"allocationCid"`
	ExtraTransferLegSides []struct{} `json:"extraTransferLegSides"`
	NextIterationFunding  *struct{}  `json:"nextIterationFunding"`
}

type settleBatchArgs struct {
	Settlement   settlementInfo    `json:"settlement"`
	TransferLegs []transferLeg     `json:"transferLegs"`
	Allocations  []batchAllocation `json:"allocations"`
	Actors       []string          `json:"actors"`
	ExtraArgs    extraArgs         `json:"extraArgs"`
}

type cancelArgs struct {
	Actors    []string  `json:"actors"`
	ExtraArgs extraArgs `json:"extraArgs"`
}

type acceptArgs struct {
	ExtraArgs extraArgs `json:"extraArgs"`
}

type transfer struct {
	Sender           string       `json:"sender"`
	Receiver         string       `json:"receiver"`
	Amount           string       `json:"amount"`
	InstrumentID     instrumentID `json:"instrumentId"`
	RequestedAt      string       `json:"requestedAt"`
	ExecuteBefore    string       `json:"executeBefore"`
	InputHoldingCids []string     `json:"inputHoldingCids"`
	Meta             metadata     `json:"meta"`
}

type transferArgs struct {
	ExpectedAdmin string    `json:"expectedAdmin"`
	Transfer      transfer  `json:"transfer"`
	ExtraArgs     extraArgs `json:"extraArgs"`
}

type partyConfig struct {
	CanInitiate bool `json:"canInitiate"`
	MustApprove bool `json:"mustApprove"`
}

type accountConfig struct {
	Admin          string      `json:"admin"`
	Account        account     `json:"account"`
	OwnerConfig    partyConfig `json:"ownerConfig"`
	ProviderConfig partyConfig `json:"providerConfig"`
}

type offerMintArgs struct {
	Receiver       account       `json:"receiver"`
	Amount         string        `json:"amount"`
	InstrumentID   instrumentID  `json:"instrumentId"`
	OfferedAt      string        `json:"offeredAt"`
	ReceiverConfig accountConfig `json:"receiverConfig"`
}

func emptyMeta() metadata {
	return metadata{Values: map[string]string{}}
}

func basicAccount(party string) account {
	return account{Owner: party}
}

// rulesContext is the choice context a registry would serve: the TokenRules contract id
// under the key TestTokenV2 reads.
func rulesContext(rules domain.Disclosed) extraArgs {
	return extraArgs{
		Context: choiceContext{Values: map[string]anyValue{
			rulesContextKey: {Tag: "AV_ContractId", Value: rules.ContractID},
		}},
		Meta: emptyMeta(),
	}
}

func settlement(operator, settlementID string) settlementInfo {
	return settlementInfo{Executors: []string{operator}, ID: settlementID, Meta: emptyMeta()}
}

// ledgerTime formats a Daml Time: UTC, microsecond precision.
func ledgerTime(t time.Time) string {
	return t.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
}

func orEmpty(cids []string) []string {
	if cids == nil {
		return []string{}
	}

	return cids
}

func command(templateID, contractID, choice string, argument any, disclosed []domain.Disclosed,
	rules domain.Disclosed,
) (domain.Command, error) {
	payload, err := json.Marshal(exerciseCommand{ExerciseCommand: exercise{
		TemplateID:     templateID,
		ContractID:     contractID,
		Choice:         choice,
		ChoiceArgument: argument,
	}})
	if err != nil {
		return domain.Command{}, fmt.Errorf("json.Marshal %s: %w", choice, err)
	}

	return domain.Command{Payload: payload, Disclosed: disclosed, SynchronizerID: rules.SynchronizerID}, nil
}

// withTokens discloses TokenRules first, then every Token.
func withTokens(rules domain.Disclosed, tokens []domain.Disclosed) []domain.Disclosed {
	disclosed := make([]domain.Disclosed, 0, len(tokens)+1)
	disclosed = append(disclosed, rules)

	for _, token := range tokens {
		if token.ContractID != rules.ContractID {
			disclosed = append(disclosed, token)
		}
	}

	return disclosed
}

// AllocateCommand funds one slot (committed: only the executor can settle or cancel it).
func (c *Canton) AllocateCommand(rules domain.Disclosed, in domain.AllocateSpec) (domain.Command, error) {
	sides := make([]transferLegSide, 0, len(in.Slot.Sides))

	for _, side := range in.Slot.Sides {
		sides = append(sides, transferLegSide{
			TransferLegID: side.LegID,
			Side:          string(side.Side),
			Otherside:     basicAccount(side.Otherside),
			Amount:        side.Amount.String(),
			InstrumentID:  in.Slot.InstrumentID,
			Meta:          emptyMeta(),
		})
	}

	args := allocateArgs{
		Settlement: settlement(in.Operator, in.SettlementID),
		Allocation: allocationSpec{
			Admin:              in.Slot.Admin,
			Authorizer:         basicAccount(in.Slot.Party),
			TransferLegSides:   sides,
			SettlementDeadline: ledgerTime(in.Deadline),
			Committed:          true,
			Meta:               emptyMeta(),
		},
		RequestedAt:      ledgerTime(in.RequestedAt),
		InputHoldingCids: orEmpty(in.Inputs),
		ExtraArgs:        rulesContext(rules),
		Actors:           []string{in.Slot.Party},
	}

	return command(allocationFactoryV2, rules.ContractID, "AllocationFactory_Allocate", args,
		[]domain.Disclosed{rules}, rules)
}

// SettleCommand settles every leg in one batch: on the sandbox the operator is the only
// instrument admin, so one SettleBatch covers the whole trade.
func (c *Canton) SettleCommand(rules domain.Disclosed, tokens []domain.Disclosed, in domain.SettleSpec,
) (domain.Command, error) {
	legs := make([]transferLeg, 0, len(in.Legs))

	for _, leg := range in.Legs {
		legs = append(legs, transferLeg{
			TransferLegID: leg.ID,
			Sender:        basicAccount(leg.Sender),
			Receiver:      basicAccount(leg.Receiver),
			Amount:        leg.Amount.String(),
			InstrumentID:  leg.Instrument,
			Meta:          emptyMeta(),
		})
	}

	allocations := make([]batchAllocation, 0, len(in.AllocationCIDs))

	for _, cid := range in.AllocationCIDs {
		allocations = append(allocations, batchAllocation{AllocationCid: cid, ExtraTransferLegSides: []struct{}{}})
	}

	args := settleBatchArgs{
		Settlement:   settlement(in.Operator, in.SettlementID),
		TransferLegs: legs,
		Allocations:  allocations,
		Actors:       []string{in.Operator},
		ExtraArgs:    rulesContext(rules),
	}

	return command(settlementFactoryV2, rules.ContractID, "SettlementFactory_SettleBatch", args,
		withTokens(rules, tokens), rules)
}

// CancelCommand releases an allocation's holdings, as its executor.
func (c *Canton) CancelCommand(rules domain.Disclosed, tokens []domain.Disclosed, allocationCID, executor string,
) (domain.Command, error) {
	args := cancelArgs{Actors: []string{executor}, ExtraArgs: rulesContext(rules)}

	return command(allocationV2, allocationCID, "Allocation_Cancel", args, withTokens(rules, tokens), rules)
}

// AcceptTransferCommand accepts a transfer offer as its receiver. The accept consumes the
// holdings the sender locked for it, which the receiver is no stakeholder of: they come
// disclosed after TokenRules.
func (c *Canton) AcceptTransferCommand(rules domain.Disclosed, inputs []domain.Disclosed, transferCID string,
) (domain.Command, error) {
	args := acceptArgs{ExtraArgs: rulesContext(rules)}

	return command(transferInstructionV1, transferCID, "TransferInstruction_Accept", args,
		withTokens(rules, inputs), rules)
}

// TransferCommand offers holdings to another party, which accepts the offer.
func (c *Canton) TransferCommand(rules domain.Disclosed, in domain.TransferSpec) (domain.Command, error) {
	args := transferArgs{
		ExpectedAdmin: in.Admin,
		Transfer: transfer{
			Sender:           in.Sender,
			Receiver:         in.Receiver,
			Amount:           in.Amount.String(),
			InstrumentID:     instrumentID{Admin: in.Admin, ID: in.InstrumentID},
			RequestedAt:      ledgerTime(in.RequestedAt),
			ExecuteBefore:    ledgerTime(in.ExecuteBefore),
			InputHoldingCids: orEmpty(in.Inputs),
			Meta:             emptyMeta(),
		},
		ExtraArgs: rulesContext(rules),
	}

	return command(transferFactoryV1, rules.ContractID, "TransferFactory_Transfer", args,
		[]domain.Disclosed{rules}, rules)
}

// OfferMintCommand mints a transfer offer to the receiver, as the instrument admin; the
// receiver config is the self-custodial shape a provider-less account takes.
func (c *Canton) OfferMintCommand(rules domain.Disclosed, in domain.OfferMintSpec) (domain.Command, error) {
	receiver := basicAccount(in.Receiver)

	args := offerMintArgs{
		Receiver:     receiver,
		Amount:       in.Amount.String(),
		InstrumentID: instrumentID{Admin: in.Admin, ID: in.InstrumentID},
		OfferedAt:    ledgerTime(in.OfferedAt),
		ReceiverConfig: accountConfig{
			Admin:          in.Admin,
			Account:        receiver,
			OwnerConfig:    partyConfig{CanInitiate: true, MustApprove: true},
			ProviderConfig: partyConfig{CanInitiate: false, MustApprove: false},
		},
	}

	return command(tokenRulesTemplate, rules.ContractID, "TokenRules_OfferMint", args,
		[]domain.Disclosed{rules}, rules)
}
