package credentialvault

import (
	"bytes"
	"testing"

	"applyflow/backend/internal/aisettings"
)

func TestAuthenticatedOwnerBoundEncryption(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	v, err := New(key, 1)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("test-only-provider-secret")
	a, err := v.Seal("alice", "credential-a", plain)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := v.Seal("alice", "credential-a", plain)
	if bytes.Equal(a.Nonce, b.Nonce) || bytes.Contains(a.Ciphertext, plain) {
		t.Fatal("encryption is deterministic or exposes plaintext")
	}
	got, err := v.Open("alice", "credential-a", a)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatal("roundtrip failed")
	}
	for _, tc := range []struct {
		name, owner, id string
		sealed          aisettings.Sealed
	}{
		{"owner", "bob", "credential-a", a}, {"credential", "alice", "credential-b", a},
		{"version", "alice", "credential-a", aisettings.Sealed{Ciphertext: a.Ciphertext, Nonce: a.Nonce, KeyVersion: 2}},
		{"nonce", "alice", "credential-a", aisettings.Sealed{Ciphertext: a.Ciphertext, Nonce: nil, KeyVersion: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := v.Open(tc.owner, tc.id, tc.sealed); err == nil {
				t.Fatal("unbound credential decrypted")
			}
		})
	}
	a.Ciphertext[0] ^= 1
	if _, err = v.Open("alice", "credential-a", a); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}
	if _, err = New(key[:16], 1); err == nil {
		t.Fatal("weak key accepted")
	}
}
