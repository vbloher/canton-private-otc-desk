package domain

import "github.com/google/uuid"

// RFQ is what the bot needs to know of an RFQ sent to one of its makers.
type RFQ struct {
	ID        uuid.UUID
	Direction string
	Base      string
	Quote     string
	Open      bool
	Quoted    bool // the maker has a quote on it, whatever its status
}
