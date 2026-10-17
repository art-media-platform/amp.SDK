package safe_test

// Close persists only what is still unsaved (SD-security-sync §20.6): every
// mutation persists at its own site, so a clean handle's Close writes
// nothing — even one opened before a sibling's install — and a read-only
// open leaves the tome's bytes as it found them.  An unsaved install's Close
// folds in the tome first (enclave_siblings_test.go).

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

const closeTestAAD = "enclave close"

func openCloseTestEnclave(t *testing.T, store safe.TomeStore, guard safe.Guard) safe.Enclave {
	t.Helper()
	enc, err := safe.OpenEnclave(context.Background(), store, guard, []byte(closeTestAAD))
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func signingKeyOnDisk(t *testing.T, store safe.TomeStore, guard safe.Guard, ringID tag.UID) bool {
	t.Helper()
	reader := openCloseTestEnclave(t, store, guard)
	defer reader.Close(context.Background())
	ref := &safe.KeyRef{Type: safe.KeyType_SigningKey}
	ref.SetKeyringID(ringID)
	_, err := reader.FetchPubKey(ref)
	return err == nil
}

// Two handles on one tome: A installs a key (durable at return); B, opened
// before the install, closes.  A's install stays on disk.
func TestEnclave_CloseKeepsSiblingInstall(t *testing.T) {
	ctx := context.Background()
	store := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "enclave.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte(closeTestAAD))
	defer guard.Close()

	handleA := openCloseTestEnclave(t, store, guard)
	handleB := openCloseTestEnclave(t, store, guard)
	ringID := tag.NewID()
	if _, err := handleA.GenerateKey(ctx, ringID, safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}); err != nil {
		t.Fatal(err)
	}
	if !signingKeyOnDisk(t, store, guard, ringID) {
		t.Fatal("premise void: A's install is not on disk at return")
	}
	if err := handleB.Close(ctx); err != nil {
		t.Fatalf("B.Close: %v", err)
	}
	if !signingKeyOnDisk(t, store, guard, ringID) {
		t.Fatal("B's Close dropped A's durable install from disk")
	}
	if err := handleA.Close(ctx); err != nil {
		t.Fatalf("A.Close: %v", err)
	}
}

// The handle that installed first closes after a sibling's later install: it
// is clean, so its Close leaves the later install on disk — the arm that
// tells the unsaved predicate from a dirty-since-open one.
func TestEnclave_InstallerCloseKeepsLaterSiblingInstall(t *testing.T) {
	ctx := context.Background()
	store := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "enclave.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte(closeTestAAD))
	defer guard.Close()

	handleA := openCloseTestEnclave(t, store, guard)
	generateSigningKey(t, handleA, tag.NewID())
	handleB := openCloseTestEnclave(t, store, guard)
	laterRing := tag.NewID()
	generateSigningKey(t, handleB, laterRing)
	if err := handleA.Close(ctx); err != nil {
		t.Fatalf("A.Close: %v", err)
	}
	if !signingKeyOnDisk(t, store, guard, laterRing) {
		t.Fatal("the installer's Close dropped the sibling's later install")
	}
	if err := handleB.Close(ctx); err != nil {
		t.Fatalf("B.Close: %v", err)
	}
}

// An open with no install writes nothing at Close: an existing tome keeps its
// bytes, and an absent one stays absent.
func TestEnclave_CloseWritesNothingWhenClean(t *testing.T) {
	ctx := context.Background()
	tomePath := filepath.Join(t.TempDir(), "enclave.tome")
	store := safe.NewLocalTomeStore(tomePath)
	guard := safe.NewFileGuard([]byte("pass"), []byte(closeTestAAD))
	defer guard.Close()

	empty := openCloseTestEnclave(t, store, guard)
	if err := empty.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tomePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an empty enclave's Close created the tome (stat: %v)", err)
	}

	writer := openCloseTestEnclave(t, store, guard)
	if _, err := writer.GenerateKey(ctx, tag.NewID(), safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(tomePath)
	if err != nil {
		t.Fatal(err)
	}

	reader := openCloseTestEnclave(t, store, guard)
	if err := reader.Close(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(tomePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("a read-only open rewrote the tome at Close")
	}
}

// An install whose persist failed stays unsaved; Close carries it to disk, and
// a Close whose persist still fails reports the error and keeps the handle
// open for a retry.
func TestEnclave_CloseCarriesUnsavedInstall(t *testing.T) {
	ctx := context.Background()
	inner := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "enclave.tome"))
	store := &faultTomeStore{inner: inner}
	guard := safe.NewFileGuard([]byte("pass"), []byte(closeTestAAD))
	defer guard.Close()

	enc := openCloseTestEnclave(t, store, guard)
	ringID := tag.NewID()
	store.failSaves = 2
	if _, err := enc.GenerateKey(ctx, ringID, safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}); err == nil {
		t.Fatal("premise void: the injected persist failure did not occur")
	}
	if err := enc.Close(ctx); err == nil {
		t.Fatal("Close reported success while the install's persist still fails")
	}
	if signingKeyOnDisk(t, inner, guard, ringID) {
		t.Fatal("premise void: the unsaved install reached disk")
	}
	if err := enc.Close(ctx); err != nil {
		t.Fatalf("Close after the store recovers: %v", err)
	}
	if !signingKeyOnDisk(t, inner, guard, ringID) {
		t.Fatal("Close did not carry the unsaved install to disk")
	}
}
