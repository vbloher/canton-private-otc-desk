package domain

import "github.com/google/uuid"

type RFQ struct {
	ID        uuid.UUID
	Direction string
	Base      string
	Quote     string
	Open      bool
	Quoted    bool
}
