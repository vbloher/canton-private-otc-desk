package canton

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const holdingFrame = `{"workflowId":"","contractEntry":{"JsActiveContract":{"createdEvent":{
	"offset":23,"nodeId":0,"contractId":"00a1f5",
	"templateId":"621a37:Splice.Testing.Tokens.TestTokenV2.Holding:Token",
	"contractKey":null,"contractKeyHash":"",
	"createArgument":{"holding":{"account":{"owner":"makerA::1220","provider":null,"id":""},
		"instrumentId":{"admin":"operator::1220","id":"CBTC"},"amount":"10.0000000000","lock":null,"meta":{"values":{}}}},
	"createdEventBlob":"",
	"interfaceViews":[{"interfaceId":"718a0f:Splice.Api.Token.HoldingV1:Holding",
		"viewStatus":{"code":0,"message":"","details":[]},
		"viewValue":{"owner":"makerA::1220","instrumentId":{"admin":"operator::1220","id":"CBTC"},
			"amount":"10.0000000000","lock":null,"meta":{"values":{}}},
		"implementationPackageId":"621a37"}],
	"witnessParties":["makerA::1220"],"signatories":["makerA::1220","operator::1220"],"observers":[],
	"createdAt":"2026-10-20T22:31:16.293822Z","packageName":"splice-test-token-v2",
	"representativePackageId":"621a37","acsDelta":true},
	"synchronizerId":"synchronizer-1::1220a9","reassignmentCounter":0}},
	"streamContinuationToken":"CgIIBhIEHvqGkQ=="}`

const errorFrame = `{"code":"OFFSET_AFTER_LEDGER_END",
	"cause":"active_at_offset offset (9999999) is after ledger end (40)",
	"correlationId":null,"traceId":"38e0c4","context":{"participant":"'sandbox'"},"resources":[],
	"errorCategory":12,"grpcCodeValue":11,"retryInfo":"1 second","definiteAnswer":null}`

func Test_ParseACSFrame(t *testing.T) {
	t.Run("active contract with interface views", func(t *testing.T) {
		contract, err := parseACSFrame([]byte(holdingFrame))
		require.NoError(t, err)
		require.NotNil(t, contract)

		require.Equal(t, "00a1f5", contract.CreatedEvent.ContractID)
		require.Equal(t, "621a37:Splice.Testing.Tokens.TestTokenV2.Holding:Token", contract.CreatedEvent.TemplateID)
		require.Equal(t, "synchronizer-1::1220a9", contract.SynchronizerID)
		require.Len(t, contract.CreatedEvent.InterfaceViews, 1)
		require.JSONEq(t, `{"owner":"makerA::1220","instrumentId":{"admin":"operator::1220","id":"CBTC"},
			"amount":"10.0000000000","lock":null,"meta":{"values":{}}}`,
			string(contract.CreatedEvent.InterfaceViews[0].ViewValue))
	})

	t.Run("frames without a contract are skipped", func(t *testing.T) {
		for _, frame := range []string{
			`{"offset":42}`,
			`{"workflowId":"","contractEntry":{"JsEmpty":{}},"streamContinuationToken":""}`,
		} {
			contract, err := parseACSFrame([]byte(frame))
			require.NoError(t, err, frame)
			require.Nil(t, contract, frame)
		}
	})

	t.Run("error frame", func(t *testing.T) {
		contract, err := parseACSFrame([]byte(errorFrame))
		require.ErrorIs(t, err, domain.ErrLedger)
		require.ErrorContains(t, err, "OFFSET_AFTER_LEDGER_END")
		require.ErrorContains(t, err, "is after ledger end (40)")
		require.Nil(t, contract)
	})

	t.Run("undecodable frame", func(t *testing.T) {
		_, err := parseACSFrame([]byte(`{"contractEntry":`))
		require.ErrorIs(t, err, domain.ErrLedger)
	})
}

const transferView = `{"originalInstructionCid":null,"transfer":{"sender":"alice::1220","receiver":"bob::1220",
	"amount":"0.4000000000","instrumentId":{"admin":"operator::1220","id":"CBTC"},
	"requestedAt":"2026-10-20T22:48:28.297203Z","executeBefore":"2026-10-21T22:48:28.297203Z",
	"inputHoldingCids":["locked-1"],"meta":{"values":{}}},
	"status":{"tag":"TransferPendingReceiverAcceptance","value":{}},"meta":{"values":{}}}`

func Test_Transfers_FromViews(t *testing.T) {
	incoming := activeContract{CreatedEvent: createdEvent{
		ContractID:     "offer-1",
		InterfaceViews: []interfaceView{{ViewValue: []byte(transferView)}},
	}}

	transfers, err := transfersOf([]activeContract{incoming}, "bob::1220")
	require.NoError(t, err)
	require.Equal(t, []domain.IncomingTransfer{{
		ContractID:   "offer-1",
		Sender:       "alice::1220",
		Admin:        "operator::1220",
		InstrumentID: "CBTC",
		Amount:       transfers[0].Amount,
	}}, transfers)
	require.Equal(t, "0.4", transfers[0].Amount.String())

	transfers, err = transfersOf([]activeContract{incoming}, "alice::1220")
	require.NoError(t, err)
	require.Empty(t, transfers)
}

const mintView = `{"originalInstructionCid":null,"transfer":{
	"sender":"cip-112_no-owner::1220000000000000000000000000000000000000000000000000000000000000abcd",
	"receiver":"bob::1220","amount":"5.0000000000","instrumentId":{"admin":"operator::1220","id":"CBTC"},
	"requestedAt":"2026-10-20T22:48:28.297203Z","executeBefore":"2026-10-27T22:48:28.297203Z",
	"inputHoldingCids":[],"meta":{"values":{"cip-112/sender.id":"cip-112/mint"}}},
	"status":{"tag":"TransferPendingReceiverAcceptance","value":{}},"meta":{"values":{}}}`

func Test_Transfers_MintHasNoSender(t *testing.T) {
	mint := activeContract{CreatedEvent: createdEvent{
		ContractID:     "mint-1",
		InterfaceViews: []interfaceView{{ViewValue: []byte(mintView)}},
	}}

	transfers, err := transfersOf([]activeContract{mint}, "bob::1220")
	require.NoError(t, err)
	require.Len(t, transfers, 1)
	require.Empty(t, transfers[0].Sender, "the placeholder is no party to show or notify")
	require.Equal(t, domain.FaucetHint, transfers[0].SenderHint())
}

func Test_Holdings_FromViews(t *testing.T) {
	contract, err := parseACSFrame([]byte(holdingFrame))
	require.NoError(t, err)

	withView := func(cid, view string) activeContract {
		c := *contract
		c.CreatedEvent.ContractID = cid
		c.CreatedEvent.InterfaceViews = []interfaceView{{ViewValue: []byte(view)}}

		return c
	}

	deadlineLock := withView("00b2", `{"owner":"makerA::1220","instrumentId":{"admin":"operator::1220","id":"CBTC"},
		"amount":"0.5000000000","lock":{"holders":["operator::1220"],"expiresAt":"2026-10-21T23:19:01.044596Z",
		"expiresAfter":null,"context":"allocation"}}`)
	foreverLock := withView("00c3", `{"owner":"makerA::1220","instrumentId":{"admin":"operator::1220","id":"CBTC"},
		"amount":"0.2500000000","lock":{"holders":["operator::1220"],"expiresAt":null,"expiresAfter":null}}`)
	failed := withView("00d4", `null`)

	holdings, err := holdingsOf([]activeContract{*contract, deadlineLock, foreverLock, failed})
	require.NoError(t, err)
	require.Len(t, holdings, 3)

	require.Equal(t, "00a1f5", holdings[0].ContractID)
	require.Equal(t, "makerA::1220", holdings[0].Owner)
	require.Equal(t, "operator::1220", holdings[0].Admin)
	require.Equal(t, "CBTC", holdings[0].InstrumentID)
	require.Equal(t, "10.0", holdings[0].Amount.String())
	require.False(t, holdings[0].Locked)

	require.Equal(t, "0.5", holdings[1].Amount.String())
	require.True(t, holdings[1].Locked)
	require.Equal(t, time.Date(2026, 10, 21, 23, 19, 1, 44596000, time.UTC), holdings[1].LockedUntil)

	require.True(t, holdings[2].Locked)
	require.True(t, holdings[2].LockedUntil.IsZero(), "locked indefinitely")
}
