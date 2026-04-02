package safe

import (
	"context"
	"fmt"
	"sync"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"google.golang.org/protobuf/proto"
)

// enclave is the runtime cryptographic session.
//
// Lifecycle:
//
//	OpenEnclave() -> loads SealedTome from TomeStore
//	              -> unwraps DEK via Guard
//	              -> decrypts KeyTome payload
//	              -> returns Enclave with live KeyTome
//
//	enclave.Close() -> generates fresh DEK
//	                -> encrypts serialized KeyTome
//	                -> wraps DEK via Guard
//	                -> saves SealedTome to TomeStore
//	                -> zeros all sensitive material
//
// The KeyTome is fully internal; callers interact via ImportKeys, GenerateKeys,
// FetchKeyInfo, and DoCryptOp.  All methods are protected by a RWMutex.
type enclave struct {
	mu    sync.RWMutex
	store TomeStore
	guard Guard
	tome  *KeyTome
	aad   []byte // additional authenticated data bound to this enclave
}

var _ Enclave = (*enclave)(nil)

// OpenEnclave starts a new cryptographic session.
//
// If the TomeStore has no existing SealedTome, a fresh empty KeyTome is created.
// The aad parameter is bound to every seal/unseal operation for domain separation.
func OpenEnclave(
	ctx context.Context,
	store TomeStore,
	guard Guard,
	aad []byte,
) (Enclave, error) {

	enc := &enclave{
		store: store,
		guard: guard,
		aad:   append([]byte(nil), aad...),
	}

	sealed, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to load SealedTome: %w", err)
	}

	if sealed == nil {
		// Fresh start — empty KeyTome
		enc.tome = &KeyTome{Revision: 1}
		return enc, nil
	}

	// Unwrap the DEK
	dek, err := guard.UnwrapDEK(ctx, sealed.WrappedDEK, enc.aad)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to unwrap DEK: %w", err)
	}
	defer Zero(dek)

	// Decrypt the KeyTome payload
	tomeBytes, err := openAEAD(dek, sealed.TomeNonce, sealed.Cipherblob, enc.aad)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to decrypt KeyTome: %w", err)
	}
	defer Zero(tomeBytes)

	enc.tome = &KeyTome{}
	if err := proto.Unmarshal(tomeBytes, enc.tome); err != nil {
		return nil, fmt.Errorf("safe: failed to unmarshal KeyTome: %w", err)
	}

	return enc, nil
}

// ImportKeys merges all keys in srcTome into this enclave's KeyTome.
// Duplicate key entries are ignored/dropped.  If there are problematic keys
// (pub-key collisions that are not exact dupes), they remain in srcTome.
//
// THREADSAFE
func (enc *enclave) ImportKeys(srcTome *KeyTome) error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	if enc.tome == nil {
		return fmt.Errorf("safe: enclave is closed")
	}
	enc.tome.MergeTome(srcTome)
	return nil
}

// GenerateKeys generates a new KeyEntry for each entry in srcTome based on KeyType and CryptoKitID.
// The newly generated keys are merged into the host KeyTome.
// A copy of each generated entry (minus PrivKey) is placed into srcTome for the caller.
//
// THREADSAFE
func (enc *enclave) GenerateKeys(srcTome *KeyTome) (*KeyTome, error) {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	if enc.tome == nil {
		return nil, fmt.Errorf("safe: enclave is closed")
	}

	// Natural key collisions are astronomically unlikely.
	// Retry as a safeguard against the impossible.
	tries := 3
	for ; tries > 0; tries-- {
		newKeys, err := srcTome.GenerateFork(RandReader, DEKSize)
		if err != nil {
			return nil, err
		}

		enc.tome.MergeTome(newKeys)
		if len(newKeys.Keyrings) == 0 {
			break
		}
	}
	if tries == 0 {
		return nil, status.Code_KeyGenerationFailed.Error("generated keys failed to merge after 3 attempts")
	}

	return srcTome, nil
}

// FetchKeyInfo returns info about the referenced key.
// If len(inKeyRef.PubKey) == 0, the newest KeyEntry in the implied Keyring is returned.
//
// THREADSAFE
func (enc *enclave) FetchKeyInfo(inKeyRef *KeyRef) (*KeyInfo, error) {
	enc.mu.RLock()
	defer enc.mu.RUnlock()

	if enc.tome == nil {
		return nil, fmt.Errorf("safe: enclave is closed")
	}

	entry, err := enc.fetchKey(inKeyRef)
	if err != nil {
		return nil, err
	}
	return entry.KeyInfo, nil
}

// DoCryptOp performs signing, symmetric encryption/decryption, and asymmetric encryption/decryption.
//
// THREADSAFE
func (enc *enclave) DoCryptOp(inArgs *CryptOpArgs) (*CryptOpOut, error) {
	enc.mu.RLock()
	defer enc.mu.RUnlock()

	if enc.tome == nil {
		return nil, fmt.Errorf("safe: enclave is closed")
	}
	if inArgs.OpKey == nil {
		return nil, status.Code_BadRequest.Error("CryptOpArgs.OpKey is required")
	}

	opKey, err := enc.fetchKey(inArgs.OpKey)
	if err != nil {
		return nil, err
	}

	kit, err := GetCryptoKit(opKey.KeyInfo.CryptoKitID)
	if err != nil {
		return nil, err
	}

	out := &CryptOpOut{
		OpPubKey: opKey.KeyInfo.PubKey,
	}

	switch inArgs.Op {

	case CryptOp_Sign:
		if kit.Sign == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support signing", kit.ID.String())
		}
		out.Output, err = kit.Sign(inArgs.Input, opKey.PrivKey)

	case CryptOp_EncryptSym:
		if kit.Encrypt == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support symmetric encryption", kit.ID.String())
		}
		out.Output, err = kit.Encrypt(RandReader, inArgs.Input, opKey.PrivKey)

	case CryptOp_DecryptSym:
		if kit.Decrypt == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support symmetric decryption", kit.ID.String())
		}
		out.Output, err = kit.Decrypt(inArgs.Input, opKey.PrivKey)

	case CryptOp_EncryptToPeer:
		if kit.EncryptFor == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support asymmetric encryption", kit.ID.String())
		}
		out.Output, err = kit.EncryptFor(RandReader, inArgs.Input, inArgs.PeerKey, opKey.PrivKey)

	case CryptOp_DecryptFromPeer:
		if kit.DecryptFrom == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support asymmetric decryption", kit.ID.String())
		}
		out.Output, err = kit.DecryptFrom(inArgs.Input, inArgs.PeerKey, opKey.PrivKey)

	default:
		return nil, status.Code_Unimplemented.Errorf("unsupported CryptOp: %v", inArgs.Op)
	}

	if err != nil {
		return nil, err
	}
	return out, nil
}

// Close seals the KeyTome and persists it, then zeros sensitive material.
func (enc *enclave) Close(ctx context.Context) error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	if enc.tome == nil {
		return nil // already closed
	}
	defer func() {
		// Zero and release the KeyTome regardless of save outcome
		enc.tome = nil
		Zero(enc.aad)
	}()

	// Serialize the KeyTome
	tomeBytes, err := proto.Marshal(enc.tome)
	if err != nil {
		return fmt.Errorf("safe: failed to marshal KeyTome: %w", err)
	}
	defer Zero(tomeBytes)

	// Generate a fresh DEK for this seal operation
	dek, err := generateDEK(RandReader)
	if err != nil {
		return err
	}
	defer Zero(dek)

	// Encrypt the serialized KeyTome
	tomeNonce, cipherblob, err := sealAEAD(RandReader, dek, tomeBytes, enc.aad)
	if err != nil {
		return fmt.Errorf("safe: failed to encrypt KeyTome: %w", err)
	}

	// Wrap the DEK under the Guard
	wrappedDEK, err := enc.guard.WrapDEK(ctx, dek, enc.aad)
	if err != nil {
		return fmt.Errorf("safe: failed to wrap DEK: %w", err)
	}

	sealed := &SealedTome{
		Version:    uint32(Const_SealedTomeVersion),
		WrappedDEK: wrappedDEK,
		Purpose:    "session",
		TomeCipher: CipherName,
		TomeNonce:  tomeNonce,
		Cipherblob: cipherblob,
	}

	if err := enc.store.Save(ctx, sealed); err != nil {
		return fmt.Errorf("safe: failed to save SealedTome: %w", err)
	}
	return nil
}

// fetchKey looks up a key in the internal KeyTome.
// Must be called with enc.mu held (read or write).
func (enc *enclave) fetchKey(ref *KeyRef) (*KeyEntry, error) {
	keyringID := ref.KeyringID()

	kr := enc.tome.FetchKeyring(keyringID)
	if kr == nil || len(kr.Keys) == 0 {
		return nil, status.Code_KeyringNotFound.Errorf("keyring %v not found", keyringID)
	}

	var match *KeyEntry
	if len(ref.PubKey) == 0 {
		match = kr.FetchNewestKey()
	} else {
		match = kr.FetchKeyWithPrefix(ref.PubKey)
	}

	if match == nil {
		return nil, status.Code_KeyringNotFound.Errorf("key not found in keyring %v (pubKey prefix: %x)", keyringID, ref.PubKey)
	}
	return match, nil
}
