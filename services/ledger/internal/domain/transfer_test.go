package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func Test_IncomingTransfer_SenderHint(t *testing.T) {
	require.Equal(t, "makerA", domain.IncomingTransfer{Sender: "makerA::1220aa"}.SenderHint())

	require.Equal(t, "faucet", domain.IncomingTransfer{Sender: ""}.SenderHint())
}
