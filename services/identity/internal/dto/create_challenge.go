package dto

type CreateChallengeInput struct {
	CredentialID string
}

type CreateChallengeOutput struct {
	Challenge     string
	EncryptedBlob string
	PublicKey     []byte
}
