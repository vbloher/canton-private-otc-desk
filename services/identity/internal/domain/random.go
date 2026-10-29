package domain

import "crypto/rand"

func randomBytes(n int) []byte {
	b := make([]byte, n)

	_, _ = rand.Read(b)

	return b
}
