package domain

import "crypto/rand"

// randomBytes never fails: crypto/rand.Read aborts the process when the system source is broken.
func randomBytes(n int) []byte {
	b := make([]byte, n)

	_, _ = rand.Read(b)

	return b
}
