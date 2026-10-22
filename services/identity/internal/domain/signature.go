package domain

import "crypto/ed25519"

const (
	PublicKeySize = ed25519.PublicKeySize
	SignatureSize = ed25519.SignatureSize
)

// VerifySignature checks an Ed25519 signature; wrong lengths are a failed check, not a panic.
func VerifySignature(publicKey, message, signature []byte) bool {
	if len(publicKey) != PublicKeySize || len(signature) != SignatureSize {
		return false
	}

	return ed25519.Verify(publicKey, message, signature)
}

// ValidateSignature rejects anything that cannot be a raw Ed25519 signature.
func ValidateSignature(signature []byte) error {
	if len(signature) != SignatureSize {
		return Invalidf("signature must be %d bytes", SignatureSize)
	}

	return nil
}
