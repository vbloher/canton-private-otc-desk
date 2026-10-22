package usecase_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func mintSpec(instrument, amount string) any {
	return mock.MatchedBy(func(s domain.OfferMintSpec) bool {
		return s.Admin == operator && s.Receiver == alice && s.InstrumentID == instrument &&
			s.Amount.String() == amount && s.OfferedAt.Before(time.Now().Add(-30*time.Second))
	})
}

func faucetCommandID() any {
	return mock.MatchedBy(func(id string) bool { return strings.HasPrefix(id, "faucet:") })
}

func Test_Faucet_OffersThenQueuesAccepts(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.cachedRules()

	// One mint offer per FAUCET_AMOUNTS entry (CBTC and USDCx; CETH has none), as the operator.
	d.canton.EXPECT().OfferMintCommand(rules, mintSpec("CBTC", "1.0")).Return(command("mint-cbtc"), nil).Once()
	d.canton.EXPECT().OfferMintCommand(rules, mintSpec("USDCx", "100000.0")).Return(command("mint-usdcx"), nil).Once()
	mintCBTC := d.canton.EXPECT().Submit(Any, operator, faucetCommandID(), command("mint-cbtc")).
		Return(domain.Transaction{UpdateID: "u-1", Created: []string{"offer-cbtc"}}, nil).Once()
	mintUSDCx := d.canton.EXPECT().Submit(Any, operator, faucetCommandID(), command("mint-usdcx")).
		Return(domain.Transaction{UpdateID: "u-2", Created: []string{"offer-usdcx"}}, nil).Once()

	// Then alice accepts each offer with her own key.
	d.canton.EXPECT().AcceptTransferCommand(rules, noInputs, "offer-cbtc").Return(command("accept-cbtc"), nil).Once().
		NotBefore(mintCBTC, mintUSDCx)
	d.canton.EXPECT().AcceptTransferCommand(rules, noInputs, "offer-usdcx").Return(command("accept-usdcx"), nil).Once().
		NotBefore(mintCBTC, mintUSDCx)
	d.canton.EXPECT().Prepare(Any, alice, "accept:offer-cbtc", command("accept-cbtc")).
		Return(prepared("hash-cbtc"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "accept:offer-usdcx", command("accept-usdcx")).
		Return(prepared("hash-usdcx"), nil).Once()
	d.postgres.EXPECT().SaveActions(Any, mock.MatchedBy(func(actions []domain.Action) bool {
		return len(actions) == 2 &&
			actions[0].Purpose == domain.PurposeTransferAccept && actions[0].Ref == "offer-cbtc" &&
			actions[1].Purpose == domain.PurposeTransferAccept && actions[1].Ref == "offer-usdcx" &&
			actions[0].Counterparty == "" // a mint offer has no sender to notify
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignRequested), parties(alice)).Return(nil).Times(2)

	output, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: alice})
	require.NoError(t, err)
	require.Len(t, output.Actions, 2)
	require.Equal(t, "hash-cbtc", string(output.Actions[0].Hash))
}

func Test_Faucet_HostedPartyAcceptsAtOnce(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.cachedRules()
	d.canton.EXPECT().OfferMintCommand(rules, Any).Return(command("mint"), nil).Times(2)
	d.canton.EXPECT().Submit(Any, operator, faucetCommandID(), command("mint")).
		Return(domain.Transaction{Created: []string{"offer"}}, nil).Times(2)
	d.canton.EXPECT().AcceptTransferCommand(rules, noInputs, "offer").Return(command("accept"), nil).Times(2)
	d.canton.EXPECT().Submit(Any, makerA, "accept:offer", command("accept")).Return(domain.Transaction{}, nil).Times(2)
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(makerA)).Return(nil).Once()

	output, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: makerA})
	require.NoError(t, err)
	require.Empty(t, output.Actions)
}
