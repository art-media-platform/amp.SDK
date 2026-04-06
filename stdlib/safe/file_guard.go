package safe

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/proto"
)

// fileGuard implements Guard using a local passphrase.
//
// The passphrase is stretched via HKDF with a random salt to produce a wrapping key,
// which then encrypts/decrypts the DEK using an AEAD cipher.
//
// This models a mobile device where the root secret is a user-derived passphrase
// stored in the OS keychain or derived from biometric unlock.
type fileGuard struct {
	rootKey []byte    // passphrase or raw root material (zeroed on Close)
	keyID   []byte    // stable identifier for this guard instance
	rand    io.Reader // crypto random source
}

var _ Guard = (*fileGuard)(nil)

// NewFileGuard creates a Guard backed by a local passphrase.
//
// rootKey is the passphrase or raw key material.
// keyID is an opaque identifier that tags every WrappedDEK produced by this guard.
func NewFileGuard(rootKey []byte, keyID []byte) Guard {
	rk := make([]byte, len(rootKey))
	copy(rk, rootKey)

	kid := make([]byte, len(keyID))
	copy(kid, keyID)

	return &fileGuard{
		rootKey: rk,
		keyID:   kid,
		rand:    RandReader,
	}
}

func (g *fileGuard) Info(_ context.Context) (*GuardInfo, error) {
	return &GuardInfo{
		Provider:       "file",
		Label:          "Local passphrase guard",
		KeyID:          g.keyID,
		HardwareBacked: false,
		Removable:      false,
		ExportableRoot: true,
	}, nil
}

func (g *fileGuard) WrapDEK(_ context.Context, dek []byte, aad []byte) (*WrappedDEK, error) {
	if len(g.rootKey) == 0 {
		return nil, fmt.Errorf("safe: fileGuard is closed")
	}

	// Generate a fresh salt for each wrap operation
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(g.rand, salt); err != nil {
		return nil, fmt.Errorf("safe: failed to generate salt: %w", err)
	}

	// Derive wrapping key from passphrase + salt
	info := []byte("safe.fileGuard.WrapDEK")
	wrappingKey, err := DeriveKey(g.rootKey, salt, info)
	if err != nil {
		return nil, err
	}
	defer Zero(wrappingKey)

	// Encrypt the DEK
	nonce, cipherblob, err := SealAEAD(g.rand, wrappingKey, dek, aad)
	if err != nil {
		return nil, err
	}

	return &WrappedDEK{
		Version:    uint32(Const_SealedTomeVersion),
		Provider:   "fileGuard",
		KeyID:      g.keyID,
		KDF:        KDFName,
		Cipher:     CipherName,
		Salt:       salt,
		Nonce:      nonce,
		Cipherblob: cipherblob,
	}, nil
}

func (g *fileGuard) UnwrapDEK(_ context.Context, wrapped *WrappedDEK, aad []byte) ([]byte, error) {
	if len(g.rootKey) == 0 {
		return nil, fmt.Errorf("safe: fileGuard is closed")
	}

	if wrapped.Provider != "fileGuard" {
		return nil, fmt.Errorf("safe: WrappedDEK provider mismatch: got %q, want \"fileGuard\"", wrapped.Provider)
	}
	if !bytesEqual(wrapped.KeyID, g.keyID) {
		return nil, fmt.Errorf("safe: WrappedDEK KeyID mismatch")
	}

	// Re-derive the wrapping key using the stored salt
	info := []byte("safe.fileGuard.WrapDEK")
	wrappingKey, err := DeriveKey(g.rootKey, wrapped.Salt, info)
	if err != nil {
		return nil, err
	}
	defer Zero(wrappingKey)

	return OpenAEAD(wrappingKey, wrapped.Nonce, wrapped.Cipherblob, aad)
}

func (g *fileGuard) Close() error {
	Zero(g.rootKey)
	g.rootKey = nil
	return nil
}

// bytesEqual performs constant-time comparison.
func bytesEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// localTomeStore implements TomeStore by reading/writing a single file.
type localTomeStore struct {
	pathname string
}

var _ TomeStore = (*localTomeStore)(nil)

// NewLocalTomeStore creates a TomeStore backed by a file at pathname.
func NewLocalTomeStore(pathname string) TomeStore {
	return &localTomeStore{
		pathname: pathname,
	}
}

func (s *localTomeStore) Load(_ context.Context) (*SealedTome, error) {
	buf, err := os.ReadFile(s.pathname)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No existing tome — fresh start
		}
		return nil, fmt.Errorf("safe: failed to read tome file %q: %w", s.pathname, err)
	}

	sealed := &SealedTome{}
	if err := proto.Unmarshal(buf, sealed); err != nil {
		return nil, fmt.Errorf("safe: failed to unmarshal SealedTome from %q: %w", s.pathname, err)
	}

	return sealed, nil
}

func (s *localTomeStore) Save(_ context.Context, sealed *SealedTome) error {
	buf, err := proto.Marshal(sealed)
	if err != nil {
		return fmt.Errorf("safe: failed to marshal SealedTome: %w", err)
	}

	if err := os.WriteFile(s.pathname, buf, 0600); err != nil {
		return fmt.Errorf("safe: failed to write tome file %q: %w", s.pathname, err)
	}

	return nil
}
