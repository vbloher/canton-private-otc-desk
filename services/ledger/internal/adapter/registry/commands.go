package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const (
	transferInstructionModule = "#splice-api-token-transfer-instruction-v1:Splice.Api.Token.TransferInstructionV1:"
	transferInstructionV1     = transferInstructionModule + "TransferInstruction"
	transferFactoryV1         = transferInstructionModule + "TransferFactory"

	allocationModule    = "#splice-api-token-allocation-v2:Splice.Api.Token.AllocationV2:"
	allocationV2        = allocationModule + "Allocation"
	settlementFactoryV2 = allocationModule + "SettlementFactory"

	allocationFactoryV2 = "#splice-api-token-allocation-instruction-v2:Splice.Api.Token.AllocationInstructionV2:" +
		"AllocationFactory"
)

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

type account struct {
	Owner    string  `json:"owner"`
	Provider *string `json:"provider"`
	ID       string  `json:"id"`
}

type extraArgs struct {
	Context json.RawMessage `json:"context"`
	Meta    metadata        `json:"meta"`
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

type instrumentID struct {
	Admin string `json:"admin"`
	ID    string `json:"id"`
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

func emptyMeta() metadata {
	return metadata{Values: map[string]string{}}
}

func emptyContext() json.RawMessage {
	return json.RawMessage(`{"values":{}}`)
}

func noContext() extraArgs {
	return extraArgs{Context: emptyContext(), Meta: emptyMeta()}
}

func withContext(c choiceContext) extraArgs {
	return extraArgs{Context: c.data(), Meta: emptyMeta()}
}

func basicAccount(party string) account {
	return account{Owner: party}
}

func settlement(operator, settlementID string) settlementInfo {
	return settlementInfo{Executors: []string{operator}, ID: settlementID, Meta: emptyMeta()}
}

func ledgerTime(t time.Time) string {
	return t.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
}

func orEmpty(cids []string) []string {
	if cids == nil {
		return []string{}
	}

	return cids
}

func endpoint(registry string, path ...string) string {
	escaped := make([]string, 0, len(path))
	for _, p := range path {
		escaped = append(escaped, url.PathEscape(p))
	}

	return registry + "/" + strings.Join(escaped, "/")
}

func payload(templateID, contractID, choice string, argument any) (json.RawMessage, error) {
	data, err := json.Marshal(exerciseCommand{ExerciseCommand: exercise{
		TemplateID:     templateID,
		ContractID:     contractID,
		Choice:         choice,
		ChoiceArgument: argument,
	}})
	if err != nil {
		return nil, fmt.Errorf("json.Marshal %s: %w", choice, err)
	}

	return data, nil
}

func command(payloads []json.RawMessage, contexts ...choiceContext) (domain.Command, error) {
	cmd := domain.Command{Commands: payloads}

	var synchronizers []string

	for _, c := range contexts {
		for _, d := range c.DisclosedContracts {
			if d.SynchronizerID != "" && !slices.Contains(synchronizers, d.SynchronizerID) {
				synchronizers = append(synchronizers, d.SynchronizerID)
			}

			if slices.ContainsFunc(cmd.Disclosed, func(x domain.Disclosed) bool { return x.ContractID == d.ContractID }) {
				continue
			}

			cmd.Disclosed = append(cmd.Disclosed, domain.Disclosed{
				TemplateID:       d.TemplateID,
				ContractID:       d.ContractID,
				CreatedEventBlob: d.CreatedEventBlob,
				SynchronizerID:   d.SynchronizerID,
			})
		}
	}

	if len(synchronizers) > 1 {
		return domain.Command{}, fmt.Errorf("%w: the registries pin one transaction to several synchronizers: %s",
			domain.ErrLedger, strings.Join(synchronizers, ", "))
	}

	if len(synchronizers) == 1 {
		cmd.SynchronizerID = synchronizers[0]
	}

	return cmd, nil
}

func single(templateID, contractID, choice string, argument any, c choiceContext) (domain.Command, error) {
	p, err := payload(templateID, contractID, choice, argument)
	if err != nil {
		return domain.Command{}, err
	}

	return command([]json.RawMessage{p}, c)
}

func (r *Registry) AllocateCommand(ctx context.Context, in domain.AllocateSpec) (domain.Command, error) {
	ctx, span := tracer.Start(ctx, "adapter registry AllocateCommand")
	defer span.End()

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
		ExtraArgs:        noContext(),
		Actors:           []string{in.Slot.Party},
	}

	f, err := r.factory(ctx, endpoint(in.Slot.Registry, "allocation-instruction", "v2", "allocation-factory"), args)
	if err != nil {
		return domain.Command{}, err
	}

	args.ExtraArgs = withContext(f.ChoiceContext)

	return single(allocationFactoryV2, f.FactoryID, "AllocationFactory_Allocate", args, f.ChoiceContext)
}

func (r *Registry) SettleCommand(ctx context.Context, in domain.SettleSpec) (domain.Command, error) {
	ctx, span := tracer.Start(ctx, "adapter registry SettleCommand")
	defer span.End()

	payloads := make([]json.RawMessage, 0, len(in.Batches))
	contexts := make([]choiceContext, 0, len(in.Batches))

	for _, batch := range in.Batches {
		args := settleBatchArgs{
			Settlement:   settlement(in.Operator, in.SettlementID),
			TransferLegs: transferLegs(batch.Legs),
			Allocations:  batchAllocations(batch.AllocationCIDs),
			Actors:       []string{in.Operator},
			ExtraArgs:    noContext(),
		}

		f, err := r.factory(ctx, endpoint(batch.Registry, "allocation", "v2", "settlement-factory"), args)
		if err != nil {
			return domain.Command{}, err
		}

		args.ExtraArgs = withContext(f.ChoiceContext)

		p, err := payload(settlementFactoryV2, f.FactoryID, "SettlementFactory_SettleBatch", args)
		if err != nil {
			return domain.Command{}, err
		}

		payloads = append(payloads, p)
		contexts = append(contexts, f.ChoiceContext)
	}

	return command(payloads, contexts...)
}

func transferLegs(legs []domain.TransferLeg) []transferLeg {
	out := make([]transferLeg, 0, len(legs))

	for _, leg := range legs {
		out = append(out, transferLeg{
			TransferLegID: leg.ID,
			Sender:        basicAccount(leg.Sender),
			Receiver:      basicAccount(leg.Receiver),
			Amount:        leg.Amount.String(),
			InstrumentID:  leg.Instrument.ID,
			Meta:          emptyMeta(),
		})
	}

	return out
}

func batchAllocations(cids []string) []batchAllocation {
	out := make([]batchAllocation, 0, len(cids))

	for _, cid := range cids {
		out = append(out, batchAllocation{AllocationCid: cid, ExtraTransferLegSides: []struct{}{}})
	}

	return out
}

func (r *Registry) CancelCommand(ctx context.Context, registry, allocationCID, executor string,
) (domain.Command, error) {
	ctx, span := tracer.Start(ctx, "adapter registry CancelCommand")
	defer span.End()

	c, err := r.choiceContext(ctx, endpoint(registry, "allocations", "v2", allocationCID, "choice-contexts", "cancel"))
	if err != nil {
		return domain.Command{}, err
	}

	args := cancelArgs{Actors: []string{executor}, ExtraArgs: withContext(c)}

	return single(allocationV2, allocationCID, "Allocation_Cancel", args, c)
}

func (r *Registry) AcceptTransferCommand(ctx context.Context, registry, transferCID string) (domain.Command, error) {
	ctx, span := tracer.Start(ctx, "adapter registry AcceptTransferCommand")
	defer span.End()

	c, err := r.choiceContext(ctx,
		endpoint(registry, "transfer-instruction", "v1", transferCID, "choice-contexts", "accept"))
	if err != nil {
		return domain.Command{}, err
	}

	return single(transferInstructionV1, transferCID, "TransferInstruction_Accept",
		acceptArgs{ExtraArgs: withContext(c)}, c)
}

func (r *Registry) TransferCommand(ctx context.Context, in domain.TransferSpec) (domain.Command, error) {
	ctx, span := tracer.Start(ctx, "adapter registry TransferCommand")
	defer span.End()

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
		ExtraArgs: noContext(),
	}

	f, err := r.factory(ctx, endpoint(in.Registry, "transfer-instruction", "v1", "transfer-factory"), args)
	if err != nil {
		return domain.Command{}, err
	}

	args.ExtraArgs = withContext(f.ChoiceContext)

	return single(transferFactoryV1, f.FactoryID, "TransferFactory_Transfer", args, f.ChoiceContext)
}
