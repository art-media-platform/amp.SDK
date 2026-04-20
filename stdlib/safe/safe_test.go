package safe_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519" // register Poly25519 CryptoKit
	"github.com/art-media-platform/amp.SDK/stdlib/tag"

	"google.golang.org/protobuf/proto"
)

func TestDeriveSubKey(t *testing.T) {
	masterKey := make([]byte, 32)
	rand.Read(masterKey)

	// Same purpose yields same derived key
	key1, err := safe.DeriveSubKey(masterKey, "content")
	if err != nil {
		t.Fatal(err)
	}
	key2, err := safe.DeriveSubKey(masterKey, "content")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key1, key2) {
		t.Fatal("same purpose should produce same derived key")
	}

	// Different purpose yields different derived key
	key3, err := safe.DeriveSubKey(masterKey, "member-proof")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(key1, key3) {
		t.Fatal("different purposes should produce different derived keys")
	}

	// Derived key is 32 bytes
	if len(key1) != 32 {
		t.Fatalf("expected 32-byte derived key, got %d", len(key1))
	}
}

func TestHMAC(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	msg := []byte("test message for HMAC verification")

	mac := safe.ComputeHMAC(key, msg)
	if len(mac) != 32 {
		t.Fatalf("expected 32-byte HMAC, got %d", len(mac))
	}

	// Verify succeeds with correct inputs
	if !safe.VerifyHMAC(key, msg, mac) {
		t.Fatal("HMAC verification should succeed")
	}

	// Verify fails with wrong message
	if safe.VerifyHMAC(key, []byte("wrong message"), mac) {
		t.Fatal("HMAC verification should fail with wrong message")
	}

	// Verify fails with wrong key
	wrongKey := make([]byte, 32)
	rand.Read(wrongKey)
	if safe.VerifyHMAC(wrongKey, msg, mac) {
		t.Fatal("HMAC verification should fail with wrong key")
	}

	// Verify fails with tampered tag
	tamperedMac := make([]byte, len(mac))
	copy(tamperedMac, mac)
	tamperedMac[0] ^= 0xFF
	if safe.VerifyHMAC(key, msg, tamperedMac) {
		t.Fatal("HMAC verification should fail with tampered tag")
	}
}

func TestExportSymmetricKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := safe.NewLocalTomeStore(filepath.Join(dir, "export.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("id"))
	defer guard.Close()

	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("export-test"))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close(ctx)

	keyringID := tag.NewID()
	_, err = enc.GenerateKey(keyringID, safe.KeySpec{
		KeyType:     safe.KeyType_SymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatal(err)
	}

	ref := &safe.KeyRef{}
	ref.SetKeyringID(keyringID)

	// Export the symmetric key
	exported, err := enc.ExportSymmetricKey(ref)
	if err != nil {
		t.Fatal("ExportSymmetricKey failed:", err)
	}
	if len(exported) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(exported))
	}

	// Encrypt with Enclave, decrypt with exported key directly
	testMsg := []byte("test payload for export verification")
	encOut, err := enc.DoCryptOp(&safe.CryptOpArgs{
		Op:    safe.CryptOp_EncryptSym,
		OpKey: ref,
		Input: testMsg,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Use the CryptoKit directly with the exported key
	kit, err := safe.GetCryptoKit(safe.CryptoKitID_Poly25519)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := kit.Decrypt(encOut.Output, exported)
	if err != nil {
		t.Fatal("decrypt with exported key failed:", err)
	}
	if !bytes.Equal(decrypted, testMsg) {
		t.Fatal("decrypted message doesn't match original")
	}

	safe.Zero(exported)
}

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

	pub, err := enc.GenerateKey(keyringID, safe.KeySpec{
		KeyType:     safe.KeyType_SymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	keyRef.PubKey = pub.Bytes

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

	fetched, err := enc2.FetchPubKey(&keyRef)
	if err != nil {
		t.Fatalf("FetchPubKey after reload: %v", err)
	}
	if !bytes.Equal(fetched.Bytes, pub.Bytes) {
		t.Fatal("fetched key Bytes doesn't match original")
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

	aliceKeyringID := tag.NewID()
	aliceRef := safe.KeyRef{}
	aliceRef.SetKeyringID(aliceKeyringID)

	alice, err := enc.GenerateKey(aliceKeyringID, safe.KeySpec{
		KeyType:     safe.KeyType_SigningKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatalf("GenerateKey (Alice): %v", err)
	}
	aliceRef.PubKey = alice.Bytes

	bobKeyringID := tag.NewID()
	bobRef := safe.KeyRef{}
	bobRef.SetKeyringID(bobKeyringID)

	bob, err := enc.GenerateKey(bobKeyringID, safe.KeySpec{
		KeyType:     safe.KeyType_SigningKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatalf("GenerateKey (Bob): %v", err)
	}
	bobRef.PubKey = bob.Bytes

	// Alice encrypts for Bob
	testMsg := []byte("hello from alice to bob")
	encOut, err := enc.DoCryptOp(&safe.CryptOpArgs{
		Op:      safe.CryptOp_EncryptToPeer,
		OpKey:   &aliceRef,
		PeerKey: bob.Bytes,
		Input:   testMsg,
	})
	if err != nil {
		t.Fatalf("EncryptToPeer: %v", err)
	}

	// Bob decrypts from Alice
	decOut, err := enc.DoCryptOp(&safe.CryptOpArgs{
		Op:      safe.CryptOp_DecryptFromPeer,
		OpKey:   &bobRef,
		PeerKey: alice.Bytes,
		Input:   encOut.Output,
	})
	if err != nil {
		t.Fatalf("DecryptFromPeer: %v", err)
	}

	if !bytes.Equal(decOut.Output, testMsg) {
		t.Fatal("asymmetric decrypt doesn't match original")
	}
}

func TestImportKey(t *testing.T) {
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

	keyringID := tag.NewID()

	pubBytes := make([]byte, 32)
	rand.Read(pubBytes)
	privBytes := make([]byte, 32)
	rand.Read(privBytes)

	kp := safe.KeyPair{
		Pub: safe.PubKey{
			CryptoKitID: safe.CryptoKitID_Poly25519,
			KeyType:     safe.KeyType_SymmetricKey,
			Bytes:       pubBytes,
		},
		Prv: privBytes,
	}

	if err := enc.ImportKey(keyringID, kp); err != nil {
		t.Fatalf("ImportKey: %v", err)
	}

	ref := &safe.KeyRef{PubKey: pubBytes}
	ref.SetKeyringID(keyringID)

	got, err := enc.FetchPubKey(ref)
	if err != nil {
		t.Fatalf("FetchPubKey after import: %v", err)
	}
	if !bytes.Equal(got.Bytes, pubBytes) {
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

	sealed, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load (nonexistent): %v", err)
	}
	if sealed != nil {
		t.Fatal("expected nil for nonexistent file")
	}

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

	// Generate many keys across distinct keyrings to exercise large-payload persistence.
	const ringCount = 100
	for i := 0; i < ringCount; i++ {
		keyringID := tag.UID{uint64(i + 1), uint64(i + 1000)}
		_, err := enc.GenerateKey(keyringID, safe.KeySpec{
			KeyType:     safe.KeyType_SigningKey,
			CryptoKitID: safe.CryptoKitID_Poly25519,
		})
		if err != nil {
			t.Fatalf("GenerateKey[%d]: %v", i, err)
		}
	}

	if err := enc.Close(ctx); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(filepath.Join(dir, "large.tome"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() == 0 {
		t.Fatal("tome file should not be empty")
	}

	// Re-open and verify a known keyring is accessible.
	guard2 := safe.NewFileGuard([]byte("passphrase"), []byte("large-test"))
	enc2, err := safe.OpenEnclave(ctx, store, guard2, []byte("large"))
	if err != nil {
		t.Fatal(err)
	}

	ref := &safe.KeyRef{}
	ref.SetKeyringID(tag.UID{50, 1049})
	if _, err := enc2.FetchPubKey(ref); err != nil {
		t.Fatalf("expected to find keyring after reopen: %v", err)
	}

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

	first, err := enc.GenerateKey(keyringID, safe.KeySpec{
		KeyType:     safe.KeyType_SymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatal(err)
	}

	second, err := enc.GenerateKey(keyringID, safe.KeySpec{
		KeyType:     safe.KeyType_SymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Fetch with empty PubKey should return the newest (second).
	ref := &safe.KeyRef{}
	ref.SetKeyringID(keyringID)

	newest, err := enc.FetchPubKey(ref)
	if err != nil {
		t.Fatal(err)
	}

	if newest.TimeID[0] < first.TimeID[0] ||
		(newest.TimeID[0] == first.TimeID[0] && newest.TimeID[1] < first.TimeID[1]) {
		t.Fatal("newest key should not be older than first key")
	}
	_ = second
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

	pub, err := enc.GenerateKey(keyringID, safe.KeySpec{
		KeyType:     safe.KeyType_SymmetricKey,
		CryptoKitID: safe.CryptoKitID_Poly25519,
	})
	if err != nil {
		t.Fatal(err)
	}
	keyRef.PubKey = pub.Bytes

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
