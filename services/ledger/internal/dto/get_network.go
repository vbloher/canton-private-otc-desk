package dto

import (
	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

type Drip struct {
	Symbol string
	Amount decimal.Decimal
}

type GetNetworkOutput struct {
	Network domain.Network
	Faucet  []Drip
}
