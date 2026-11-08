package outbox

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	HeaderEventID   = "event-id"
	HeaderEventType = "event-type"
)

type Message struct {
	ID      uuid.UUID
	Topic   string
	Key     string
	Type    string
	Payload []byte
}

func NewMessage(topic, key, eventType string, payload any) (Message, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Message{}, fmt.Errorf("json.Marshal: %w", err)
	}

	return Message{ID: uuid.New(), Topic: topic, Key: key, Type: eventType, Payload: body}, nil
}
