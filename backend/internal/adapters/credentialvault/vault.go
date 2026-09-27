// Package credentialvault encrypts personal provider keys with owner-bound AES-GCM.
package credentialvault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"

	"applyflow/backend/internal/aisettings"
)

var errInvalid = errors.New("credential encryption unavailable")

type Vault struct {
	aead    cipher.AEAD
	version int
}

func New(key []byte, version int) (*Vault, error) {
	if len(key) != 32 || version < 1 {
		return nil, errInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errInvalid
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errInvalid
	}
	return &Vault{aead: aead, version: version}, nil
}
func (v *Vault) aad(owner, id string) []byte {
	b, _ := json.Marshal([]any{"applyflow-ai-v1", owner, id, v.version})
	return b
}
func (v *Vault) Seal(owner, id string, plaintext []byte) (aisettings.Sealed, error) {
	if owner == "" || id == "" || len(plaintext) < 1 || len(plaintext) > 4096 {
		return aisettings.Sealed{}, errInvalid
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return aisettings.Sealed{}, errInvalid
	}
	return aisettings.Sealed{Ciphertext: v.aead.Seal(nil, nonce, plaintext, v.aad(owner, id)), Nonce: nonce, KeyVersion: v.version}, nil
}
func (v *Vault) Open(owner, id string, sealed aisettings.Sealed) ([]byte, error) {
	if sealed.KeyVersion != v.version || len(sealed.Nonce) != v.aead.NonceSize() {
		return nil, errInvalid
	}
	out, err := v.aead.Open(nil, sealed.Nonce, sealed.Ciphertext, v.aad(owner, id))
	if err != nil {
		return nil, errInvalid
	}
	return out, nil
}
