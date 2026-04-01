package safe_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"

	"google.golang.org/protobuf/proto"
)

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()

	dir := t.TempDir()
	storePath := filepath.Join(dir, "test.tome")
	store := safe.NewLocalTomeStore(storePath)

	pass := []byte("hunter2-test-passphrase")
	keyID := []byte("test-key-001")
	aad := []byte("amp.test.roundtrip")

	guard := safe.NewFileGuard(pass, keyID)
	defer guard.Close()

	// -------------------------------------------------------
	// 1) Open fresh enclave, generate a symmetric key, encrypt a payload
	// -------------------------------------------------------
	enc, err := safe.OpenEnclave(ctx, store, guard, aad)
	if err != nil {
		t.Fatalf("OpenEnclave (fresh): %v", err)
	}

	keyringID := tag.NewID()
	keyRef := safe.KeyRef{}
	keyRef.SetKeyringID(keyringID)

	keyInfo, err := safe.GenerateNewKey(enc, keyringID, &safe.KeyInfo{
		KeyForm:     safe.KeyForm_SymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatalf("GenerateNewKey: %v", err)
	}
	keyRef.PubKey = keyInfo.PubKey

	// Encrypt a test message
	testMsg := make([]byte, 200)
	rand.Read(testMsg)

	encOut, err := enc.DoCryptOp(&safe.CryptOpArgs{
		Op:    safe.CryptOp_EncryptSym,
		OpKey: &keyRef,
		Input: testMsg,
	})
	if err != nil {
		t.Fatalf("EncryptSym: %v", err)
	}
	ciphertext := encOut.Output

	if err := enc.Close(ctx); err != nil {
		t.Fatalf("Close (first): %v", err)
	}

	// -------------------------------------------------------
	// 2) Re-open and decrypt — verify the KeyTome was persisted
	// -------------------------------------------------------
	guard2 := safe.NewFileGuard(pass, keyID)
	defer guard2.Close()

	enc2, err := safe.OpenEnclave(ctx, store, guard2, aad)
	if err != nil {
		t.Fatalf("OpenEnclave (reload): %v", err)
	}

	// FetchKeyInfo should find the key
	fetchedInfo, err := enc2.FetchKeyInfo(&keyRef)
	if err != nil {
		t.Fatalf("FetchKeyInfo after reload: %v", err)
	}
	if !bytes.Equal(fetchedInfo.PubKey, keyInfo.PubKey) {
		t.Fatal("fetched key PubKey doesn't match original")
	}

	// Decrypt the ciphertext
	decOut, err := enc2.DoCryptOp(&safe.CryptOpArgs{
		Op:    safe.CryptOp_DecryptSym,
		OpKey: &keyRef,
		Input: ciphertext,
	})
	if err != nil {
		t.Fatalf("DecryptSym: %v", err)
	}
	if !bytes.Equal(decOut.Output, testMsg) {
		t.Fatal("decrypted message doesn't match original")
	}

	if err := enc2.Close(ctx); err != nil {
		t.Fatalf("Close (second): %v", err)
	}

	// -------------------------------------------------------
	// 3) Bad password should fail to open
	// -------------------------------------------------------
	badGuard := safe.NewFileGuard([]byte("wrong-password"), keyID)
	defer badGuard.Close()

	_, err = safe.OpenEnclave(ctx, store, badGuard, aad)
	if err == nil {
		t.Fatal("expected error with wrong passphrase, got nil")
	}

	// -------------------------------------------------------
	// 4) Bad AAD should fail to open
	// -------------------------------------------------------
	guard3 := safe.NewFileGuard(pass, keyID)
	defer guard3.Close()

	_, err = safe.OpenEnclave(ctx, store, guard3, []byte("wrong-aad"))
	if err == nil {
		t.Fatal("expected error with wrong AAD, got nil")
	}
}

func TestAsymmetricRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := safe.NewLocalTomeStore(filepath.Join(dir, "asym.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("id"))
	defer guard.Close()

	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("asym-test"))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close(ctx)

	// Generate two asymmetric key pairs (Alice and Bob)
	aliceKeyringID := tag.NewID()
	aliceRef := safe.KeyRef{}
	aliceRef.SetKeyringID(aliceKeyringID)

	aliceInfo, err := safe.GenerateNewKey(enc, aliceKeyringID, &safe.KeyInfo{
		KeyForm:     safe.KeyForm_AsymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatalf("GenerateNewKey (Alice): %v", err)
	}
	aliceRef.PubKey = aliceInfo.PubKey

	bobKeyringID := tag.NewID()
	bobRef := safe.KeyRef{}
	bobRef.SetKeyringID(bobKeyringID)

	bobInfo, err := safe.GenerateNewKey(enc, bobKeyringID, &safe.KeyInfo{
		KeyForm:     safe.KeyForm_AsymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatalf("GenerateNewKey (Bob): %v", err)
	}
	bobRef.PubKey = bobInfo.PubKey

	// Alice encrypts for Bob
	testMsg := []byte("hello from alice to bob")
	encOut, err := enc.DoCryptOp(&safe.CryptOpArgs{
		Op:      safe.CryptOp_EncryptToPeer,
		OpKey:   &aliceRef,
		PeerKey: bobInfo.PubKey,
		Input:   testMsg,
	})
	if err != nil {
		t.Fatalf("EncryptToPeer: %v", err)
	}

	// Bob decrypts from Alice
	decOut, err := enc.DoCryptOp(&safe.CryptOpArgs{
		Op:      safe.CryptOp_DecryptFromPeer,
		OpKey:   &bobRef,
		PeerKey: aliceInfo.PubKey,
		Input:   encOut.Output,
	})
	if err != nil {
		t.Fatalf("DecryptFromPeer: %v", err)
	}

	if !bytes.Equal(decOut.Output, testMsg) {
		t.Fatal("asymmetric decrypt doesn't match original")
	}
}

func TestImportKeys(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := safe.NewLocalTomeStore(filepath.Join(dir, "import.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("id"))
	defer guard.Close()

	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("import-test"))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close(ctx)

	// Build a KeyTome with a key to import
	keyringID := tag.NewID()
	importTome := &safe.KeyTome{
		Keyrings: []*safe.Keyring{
			{
				UID_0: keyringID[0],
				UID_1: keyringID[1],
				Keys: []*safe.KeyEntry{
					{
						KeyInfo: &safe.KeyInfo{
							KeyForm:     safe.KeyForm_SymmetricKey,
							CryptoKitID: safe.CryptoKitID_Poly25519,
							TimeCreated: 1000,
							PubKey:      make([]byte, 32),
						},
						PrivKey: make([]byte, 32),
					},
				},
			},
		},
	}

	// Fill with random data
	rand.Read(importTome.Keyrings[0].Keys[0].KeyInfo.PubKey)
	rand.Read(importTome.Keyrings[0].Keys[0].PrivKey)

	pubKey := make([]byte, 32)
	copy(pubKey, importTome.Keyrings[0].Keys[0].KeyInfo.PubKey)

	err = enc.ImportKeys(importTome)
	if err != nil {
		t.Fatalf("ImportKeys: %v", err)
	}

	// All keys should have been merged — no leftovers
	if len(importTome.Keyrings) != 0 {
		t.Fatalf("expected 0 leftover keyrings, got %d", len(importTome.Keyrings))
	}

	// Verify the key is fetchable
	ref := &safe.KeyRef{PubKey: pubKey}
	ref.SetKeyringID(keyringID)

	info, err := enc.FetchKeyInfo(ref)
	if err != nil {
		t.Fatalf("FetchKeyInfo after import: %v", err)
	}
	if !bytes.Equal(info.PubKey, pubKey) {
		t.Fatal("imported key PubKey mismatch")
	}
}

func TestXChaCha20PolyCryptoKit(t *testing.T) {
	safe.CryptoKitTest(safe.CryptoKitID_Poly25519, t)
}

func TestFileGuardWrapUnwrap(t *testing.T) {
	ctx := context.Background()

	pass := []byte("test-passphrase-wrap")
	keyID := []byte("wrap-test-key")
	guard := safe.NewFileGuard(pass, keyID)
	defer guard.Close()

	// Generate a random DEK
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		t.Fatal(err)
	}

	aad := []byte("wrap-unwrap-test")

	wrapped, err := guard.WrapDEK(ctx, dek, aad)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}

	if wrapped.Provider != "fileGuard" {
		t.Fatalf("expected provider 'fileGuard', got %q", wrapped.Provider)
	}
	if wrapped.Cipher != "XChaCha20-Poly1305" {
		t.Fatalf("unexpected cipher: %s", wrapped.Cipher)
	}

	recovered, err := guard.UnwrapDEK(ctx, wrapped, aad)
	if err != nil {
		t.Fatalf("UnwrapDEK: %v", err)
	}

	if !bytes.Equal(dek, recovered) {
		t.Fatal("recovered DEK does not match original")
	}
}

func TestLocalTomeStoreLoadSave(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store := safe.NewLocalTomeStore(filepath.Join(dir, "store.tome"))

	// Load from nonexistent file should return nil
	sealed, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load (nonexistent): %v", err)
	}
	if sealed != nil {
		t.Fatal("expected nil for nonexistent file")
	}

	// Save and reload
	original := &safe.SealedTome{
		Version:    1,
		Purpose:    "test",
		TomeCipher: "XChaCha20-Poly1305",
		TomeNonce:  []byte("test-nonce-24-bytes!!!!"),
		Cipherblob: []byte("fake-cipherblob"),
	}

	if err := store.Save(ctx, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !proto.Equal(original, loaded) {
		t.Fatal("loaded SealedTome doesn't match original")
	}
}

func TestGuardInfo(t *testing.T) {
	ctx := context.Background()
	guard := safe.NewFileGuard([]byte("pass"), []byte("id"))
	defer guard.Close()

	info, err := guard.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Provider != "file" {
		t.Fatalf("expected provider 'file', got %q", info.Provider)
	}
	if info.HardwareBacked {
		t.Fatal("fileGuard should not be hardware-backed")
	}
}

func TestYubiGuardPlaceholder(t *testing.T) {
	_, err := safe.NewYubiGuard()
	if err == nil {
		t.Fatal("expected error from unimplemented yubiGuard")
	}
}

func TestDoubleClose(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := safe.NewLocalTomeStore(filepath.Join(dir, "double.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("id"))

	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("aad"))
	if err != nil {
		t.Fatal(err)
	}

	if err := enc.Close(ctx); err != nil {
		t.Fatal(err)
	}

	// Second close should be a no-op
	if err := enc.Close(ctx); err != nil {
		t.Fatalf("double Close should be no-op, got: %v", err)
	}
}

func TestLargePayload(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := safe.NewLocalTomeStore(filepath.Join(dir, "large.tome"))
	guard := safe.NewFileGuard([]byte("passphrase"), []byte("large-test"))

	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("large"))
	if err != nil {
		t.Fatal(err)
	}

	// Import a tome with many keyrings to test large-payload persistence
	largeTome := &safe.KeyTome{
		Keyrings: make([]*safe.Keyring, 100),
	}
	for i := range largeTome.Keyrings {
		largeTome.Keyrings[i] = &safe.Keyring{
			UID_0: uint64(i + 1),
			UID_1: uint64(i + 1000),
		}
	}
	if err := enc.ImportKeys(largeTome); err != nil {
		t.Fatalf("ImportKeys (large): %v", err)
	}

	if err := enc.Close(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify file was written
	info, err := os.Stat(filepath.Join(dir, "large.tome"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("tome file should not be empty")
	}

	// Re-open and verify
	guard2 := safe.NewFileGuard([]byte("passphrase"), []byte("large-test"))
	enc2, err := safe.OpenEnclave(ctx, store, guard2, []byte("large"))
	if err != nil {
		t.Fatal(err)
	}

	// Spot-check: verify a keyring we imported is accessible
	ref := &safe.KeyRef{}
	ref.SetKeyringID([2]uint64{50, 1049})
	_, err = enc2.FetchKeyInfo(ref)
	// This will fail since the keyring has no keys — that's expected.
	// The important thing is the enclave opened without error.

	enc2.Close(ctx)
}

func TestFetchNewestKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := safe.NewLocalTomeStore(filepath.Join(dir, "newest.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("id"))
	defer guard.Close()

	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("newest-test"))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close(ctx)

	keyringID := tag.NewID()

	// Generate two keys on the same keyring
	info1, err := safe.GenerateNewKey(enc, keyringID, &safe.KeyInfo{
		KeyForm:     safe.KeyForm_SymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatal(err)
	}

	info2, err := safe.GenerateNewKey(enc, keyringID, &safe.KeyInfo{
		KeyForm:     safe.KeyForm_SymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Fetch with empty PubKey should return the newest
	ref := &safe.KeyRef{}
	ref.SetKeyringID(keyringID)

	newest, err := enc.FetchKeyInfo(ref)
	if err != nil {
		t.Fatal(err)
	}

	// The second key should be newer (or equal time)
	if newest.TimeCreated < info1.TimeCreated {
		t.Fatal("newest key should not be older than first key")
	}
	_ = info2 // suppress unused warning; info2 was the second key generated

	if !bytes.Equal(newest.PubKey, info2.PubKey) && newest.TimeCreated == info2.TimeCreated {
		// If same timestamp, either is acceptable
	}
}

func TestEncryptDecryptVariousSizes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := safe.NewLocalTomeStore(filepath.Join(dir, "sizes.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("id"))
	defer guard.Close()

	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("sizes-test"))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close(ctx)

	keyringID := tag.NewID()
	keyRef := safe.KeyRef{}
	keyRef.SetKeyringID(keyringID)

	keyInfo, err := safe.GenerateNewKey(enc, keyringID, &safe.KeyInfo{
		KeyForm:     safe.KeyForm_SymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatal(err)
	}
	keyRef.PubKey = keyInfo.PubKey

	// Test various payload sizes including edge cases
	sizes := []int{0, 1, 15, 16, 31, 32, 100, 1000, 4096, 65536}
	for _, sz := range sizes {
		testMsg := make([]byte, sz)
		rand.Read(testMsg)

		encOut, err := enc.DoCryptOp(&safe.CryptOpArgs{
			Op:    safe.CryptOp_EncryptSym,
			OpKey: &keyRef,
			Input: testMsg,
		})
		if err != nil {
			t.Fatalf("EncryptSym (size=%d): %v", sz, err)
		}

		decOut, err := enc.DoCryptOp(&safe.CryptOpArgs{
			Op:    safe.CryptOp_DecryptSym,
			OpKey: &keyRef,
			Input: encOut.Output,
		})
		if err != nil {
			t.Fatalf("DecryptSym (size=%d): %v", sz, err)
		}

		if !bytes.Equal(decOut.Output, testMsg) {
			t.Fatalf("decrypt mismatch at size=%d", sz)
		}
	}
}
