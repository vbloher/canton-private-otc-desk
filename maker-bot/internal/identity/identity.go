package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

var ErrCorrupt = errors.New("identity file is corrupt")

type Identity struct {
	PartyID        string
	Hint           string
	RegistrationID uuid.UUID
	MultiHash      []byte
	Complete       bool
	Key            ed25519.PrivateKey
}

type file struct {
	PartyID        string    `json:"partyId"`
	Hint           string    `json:"hint"`
	RegistrationID uuid.UUID `json:"registrationId"`
	MultiHash      []byte    `json:"multiHash"`
	Complete       bool      `json:"complete"`
	PrivateKey     []byte    `json:"privateKey"`
}

func Generate() (Identity, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, fmt.Errorf("ed25519.GenerateKey: %w", err)
	}

	return Identity{Key: key}, nil
}

func Load(path string) (Identity, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Identity{}, fmt.Errorf("os.ReadFile: %w", err)
	}

	var f file

	err = json.Unmarshal(raw, &f)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %s: %w", ErrCorrupt, path, err)
	}

	if len(f.PrivateKey) != ed25519.SeedSize {
		return Identity{}, fmt.Errorf("%w: %s holds no Ed25519 key", ErrCorrupt, path)
	}

	return Identity{
		PartyID:        f.PartyID,
		Hint:           f.Hint,
		RegistrationID: f.RegistrationID,
		MultiHash:      f.MultiHash,
		Complete:       f.Complete,
		Key:            ed25519.NewKeyFromSeed(f.PrivateKey),
	}, nil
}

func Save(path string, id Identity) error {
	raw, err := json.MarshalIndent(file{
		PartyID:        id.PartyID,
		Hint:           id.Hint,
		RegistrationID: id.RegistrationID,
		MultiHash:      id.MultiHash,
		Complete:       id.Complete,
		PrivateKey:     id.Key.Seed(),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("json.MarshalIndent: %w", err)
	}

	dir := filepath.Dir(path)

	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return fmt.Errorf("os.MkdirAll: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".identity-*")
	if err != nil {
		return fmt.Errorf("os.CreateTemp: %w", err)
	}

	defer func() { _ = os.Remove(tmp.Name()) }()

	_, err = tmp.Write(append(raw, '\n'))
	if err == nil {
		err = tmp.Sync()
	}

	closeErr := tmp.Close()
	if err != nil || closeErr != nil {
		return fmt.Errorf("write %s: %w", tmp.Name(), errors.Join(err, closeErr))
	}

	err = os.Rename(tmp.Name(), path)
	if err != nil {
		return fmt.Errorf("os.Rename: %w", err)
	}

	return nil
}

func (i Identity) PublicKey() ed25519.PublicKey {
	public, _ := i.Key.Public().(ed25519.PublicKey)

	return public
}

func (i Identity) Sign(message []byte) []byte {
	return ed25519.Sign(i.Key, message)
}
