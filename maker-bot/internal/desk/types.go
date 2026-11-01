package desk

import (
	"math/big"
	"time"

	"github.com/google/uuid"
)

type Registration struct {
	ID        uuid.UUID
	PartyID   string
	MultiHash []byte
}

type Me struct {
	PartyID   string
	Hint      string
	Kind      string
	MakerMode bool
}

type RFQ struct {
	ID        uuid.UUID
	TakerBuys bool
	Qty       *big.Rat
	Base      string
	Quote     string
	FeeBps    int
	Deadline  time.Time
	Open      bool
}

type Quote struct {
	ID     uuid.UUID
	Status string
}

type IncomingRFQ struct {
	RFQ     RFQ
	MyQuote *Quote
}

type Action struct {
	ID          uuid.UUID
	Purpose     string
	Description string
	Hash        []byte
}

type Transfer struct {
	ContractID string
	Sender     string
	Symbol     string
	Amount     *big.Rat
}

type Drip struct {
	Symbol string
	Amount *big.Rat
}

type Network struct {
	Name   string
	Faucet []Drip
}
