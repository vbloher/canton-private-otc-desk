package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func leg(id, sender, receiver, instrument, amount string) domain.TransferLeg {
	return domain.TransferLeg{
		ID:         id,
		Sender:     sender,
		Receiver:   receiver,
		Instrument: domain.Instrument{Symbol: instrument, ID: instrument, Admin: "operator", Registry: "https://r/" + instrument},
		Amount:     decimal.MustParse(amount),
	}
}

func keys(plan []domain.Slot) []string {
	out := make([]string, 0, len(plan))
	for _, s := range plan {
		out = append(out, s.Key())
	}

	return out
}

func sides(s domain.Slot) []string {
	out := make([]string, 0, len(s.Sides))
	for _, side := range s.Sides {
		out = append(out, fmt.Sprintf("%s %s %s %s", side.LegID, side.Side, side.Otherside, side.Amount))
	}

	return out
}

func Test_Plan_TakerBuys(t *testing.T) {
	legs := []domain.TransferLeg{
		leg("base", "makerA", "taker", "CBTC", "1.0"),
		leg("quote", "taker", "makerA", "USDCx", "100000.0"),
		leg("fee", "makerA", "operator", "USDCx", "100.0"),
	}

	plan := domain.Plan(legs)

	require.Equal(t, []string{
		"makerA|operator|CBTC",
		"makerA|operator|USDCx",
		"operator|operator|USDCx",
		"taker|operator|CBTC",
		"taker|operator|USDCx",
	}, keys(plan))

	makerCBTC := plan[0]
	require.Equal(t, []string{"base SenderSide taker 1.0"}, sides(makerCBTC))
	require.Equal(t, "1.0", makerCBTC.Send.String())

	makerUSDCx := plan[1]
	require.Equal(t, "makerA", makerUSDCx.Party)
	require.Equal(t, "operator", makerUSDCx.Admin)
	require.Equal(t, "USDCx", makerUSDCx.InstrumentID)
	require.Equal(t, []string{"quote ReceiverSide taker 100000.0", "fee SenderSide operator 100.0"}, sides(makerUSDCx))
	require.Equal(t, "100.0", makerUSDCx.Send.String())

	operatorUSDCx := plan[2]
	require.Equal(t, []string{"fee ReceiverSide makerA 100.0"}, sides(operatorUSDCx))
	require.Equal(t, "0.0", operatorUSDCx.Send.String())

	require.Equal(t, "0.0", plan[3].Send.String())
	require.Equal(t, "100000.0", plan[4].Send.String())
}

func Test_Plan_NoFeeLeg(t *testing.T) {
	legs := []domain.TransferLeg{
		leg("base", "taker", "makerB", "CETH", "2.5"),
		leg("quote", "makerB", "taker", "USDCx", "10000.0"),
	}

	plan := domain.Plan(legs)

	require.Equal(t, []string{
		"makerB|operator|CETH",
		"makerB|operator|USDCx",
		"taker|operator|CETH",
		"taker|operator|USDCx",
	}, keys(plan))

	require.Equal(t, keys(plan[2:]), keys(domain.SlotsOf(plan, "taker")))
	require.Empty(t, domain.SlotsOf(plan, "operator"))
}

func Test_Plan_ByteOrder(t *testing.T) {
	legs := []domain.TransferLeg{leg("base", "bob", "Zed", "CBTC", "1.0")}

	require.Equal(t, []string{"Zed|operator|CBTC", "bob|operator|CBTC"}, keys(domain.Plan(legs)))
}

func Test_Plan_Batches_OnePerAdmin(t *testing.T) {
	cc := domain.Instrument{Symbol: "CC", ID: "Amulet", Admin: "DSO::1", Registry: "https://scan/registry"}
	quote := leg("quote", "taker", "maker", "CC", "500000.0")
	quote.Instrument = cc
	fee := leg("fee", "maker", "operator", "CC", "100.0")
	fee.Instrument = cc
	legs := []domain.TransferLeg{leg("base", "maker", "taker", "CBTC", "1.0"), quote, fee}

	plan := domain.Plan(legs)
	require.Equal(t, []string{
		"maker|DSO::1|Amulet",
		"maker|operator|CBTC",
		"operator|DSO::1|Amulet",
		"taker|DSO::1|Amulet",
		"taker|operator|CBTC",
	}, keys(plan))
	require.Equal(t, "CC", plan[0].Symbol)
	require.Equal(t, "https://scan/registry", plan[0].Registry)

	matches := make([]domain.SlotMatch, 0, len(plan))
	for _, slot := range plan {
		matches = append(matches, domain.SlotMatch{Slot: slot, ContractID: "alloc:" + slot.Key()})
	}

	batches := domain.Batches(legs, matches)
	require.Len(t, batches, 2)

	require.Equal(t, "operator", batches[0].Admin)
	require.Equal(t, "https://r/CBTC", batches[0].Registry)
	require.Equal(t, []string{"base"}, legIDs(batches[0].Legs))
	require.Equal(t, []string{"alloc:maker|operator|CBTC", "alloc:taker|operator|CBTC"}, batches[0].AllocationCIDs)

	require.Equal(t, "DSO::1", batches[1].Admin)
	require.Equal(t, []string{"quote", "fee"}, legIDs(batches[1].Legs))
	require.Equal(t, []string{"alloc:maker|DSO::1|Amulet", "alloc:operator|DSO::1|Amulet", "alloc:taker|DSO::1|Amulet"},
		batches[1].AllocationCIDs)
}

func legIDs(legs []domain.TransferLeg) []string {
	out := make([]string, 0, len(legs))
	for _, l := range legs {
		out = append(out, l.ID)
	}

	return out
}

func errorsUnwrap(err error) error {
	return errors.Unwrap(err)
}
