package v1

import (
	"encoding/base64"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

// decodeBase64 reads a field that is sent as standard base64 (keys, signatures).
func decodeBase64(field, value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, domain.Invalidf("%s must be standard base64", field)
	}

	return decoded, nil
}

func encodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
