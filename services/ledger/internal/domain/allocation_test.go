package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func Test_MatchSlots_PartyStatus(t *testing.T) {
	plan := domain.Plan([]domain.TransferLeg{
		leg("base", "maker", "taker", "CBTC", "1.0"),
		leg("quote", "taker", "maker", "USDCx", "100000.0"),
	})

	allocs := []domain.Allocation{
		{ContractID: "maker-cbtc", SettlementID: "s-1", Authorizer: "maker", Admin: "operator", InstrumentIDs: []string{"CBTC"}},
		{ContractID: "maker-usdcx", SettlementID: "s-1", Authorizer: "maker", Admin: "operator", InstrumentIDs: []string{"USDCx"}},
		{ContractID: "taker-cbtc", SettlementID: "s-1", Authorizer: "taker", Admin: "operator", InstrumentIDs: []string{"CBTC"}},
		{ContractID: "other-settlement", SettlementID: "s-2", Authorizer: "taker", Admin: "operator", InstrumentIDs: []string{"USDCx"}},
		{ContractID: "other-admin", SettlementID: "s-1", Authorizer: "taker", Admin: "someone", InstrumentIDs: []string{"USDCx"}},
	}

	matches := domain.MatchSlots(allocs, "s-1", plan)

	cids := make([]string, 0, len(matches))
	for _, m := range matches {
		cids = append(cids, m.Key()+"="+m.ContractID)
	}

	require.Equal(t, []string{
		"maker|operator|CBTC=maker-cbtc",
		"maker|operator|USDCx=maker-usdcx",
		"taker|operator|CBTC=taker-cbtc",
		"taker|operator|USDCx=",
	}, cids)

	require.Equal(t, map[string]bool{"maker": true, "taker": false}, domain.PartyStatus(matches))
}
