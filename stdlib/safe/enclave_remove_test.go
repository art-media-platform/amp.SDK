package safe_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"

	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519"
)

func signingRef(keyringID tag.UID, pub []byte) *safe.KeyRef {
	return &safe.KeyRef{
		KeyringID_0: keyringID[0],
		KeyringID_1: keyringID[1],
		Type:        safe.KeyType_SigningKey,
		PubKey:      pub,
	}
}

// RemoveKey deletes exactly the named record: the keyring's newest pointer
// falls back to the older key, the removal is on disk at return, a prefix or
// typeless ref is refused, a key not held is KeyringNotFound, and a persist
// failure leaves the key held.
func TestEnclave_RemoveKey(t *testing.T) {
	ctx := context.Background()
	inner := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "remove.tome"))
	store := &faultTomeStore{inner: inner}
	guard := safe.NewFileGuard([]byte("pass"), []byte("remove"))
	defer guard.Close()
	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("remove-test"))
	if err != nil {
		t.Fatalf("OpenEnclave: %v", err)
	}
	ring := tag.NewID()
	spec := safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}
	older, err := enc.GenerateKey(ctx, ring, spec)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := enc.GenerateKey(ctx, ring, spec)
	if err != nil {
		t.Fatal(err)
	}
	newest, err := enc.FetchPubKey(signingRef(ring, nil))
	if err != nil || string(newest.Bytes) != string(newer.Bytes) {
		t.Fatalf("newest before removal = %x (%v); want the newer key", newest.Bytes, err)
	}

	typeless := signingRef(ring, newer.Bytes)
	typeless.Type = safe.KeyType_Unspecified
	if err := enc.RemoveKey(ctx, typeless); !status.IsError(err, status.Code_BadRequest) {
		t.Fatalf("typeless remove = %v; want BadRequest", err)
	}
	if err := enc.RemoveKey(ctx, signingRef(ring, newer.Bytes[:8])); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("prefix remove = %v; want KeyringNotFound (no prefix resolution)", err)
	}
	if _, err := enc.FetchPubKey(signingRef(ring, newer.Bytes)); err != nil {
		t.Fatalf("refused removals must leave the key held: %v", err)
	}

	store.failSaves = 1
	if err := enc.RemoveKey(ctx, signingRef(ring, newer.Bytes)); err == nil {
		t.Fatal("remove with a failing persist must error")
	}
	if _, err := enc.FetchPubKey(signingRef(ring, newer.Bytes)); err != nil {
		t.Fatalf("a failed persist must leave the key held: %v", err)
	}
	if !enc.CanSign(signingRef(ring, newer.Bytes)) {
		t.Fatal("a failed persist must leave the private half usable")
	}

	if err := enc.RemoveKey(ctx, signingRef(ring, newer.Bytes)); err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}
	if _, err := enc.FetchPubKey(signingRef(ring, newer.Bytes)); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("removed key still resolves: %v", err)
	}
	newest, err = enc.FetchPubKey(signingRef(ring, nil))
	if err != nil || string(newest.Bytes) != string(older.Bytes) {
		t.Fatalf("newest after removal = %x (%v); want the older key", newest.Bytes, err)
	}
	if err := enc.RemoveKey(ctx, signingRef(ring, newer.Bytes)); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("second remove = %v; want KeyringNotFound", err)
	}

	// Durable at return: a reopen from the tome holds only the older key.
	reopened, err := safe.OpenEnclave(ctx, inner, guard, []byte("remove-test"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close(ctx)
	if _, err := reopened.FetchPubKey(signingRef(ring, newer.Bytes)); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("removed key survived on disk: %v", err)
	}
	if !reopened.CanSign(signingRef(ring, older.Bytes)) {
		t.Fatal("the older key must survive the removal on disk")
	}

	// Removing the last key of a ring leaves no ring.
	if err := enc.RemoveKey(ctx, signingRef(ring, older.Bytes)); err != nil {
		t.Fatalf("remove last: %v", err)
	}
	if _, err := enc.FetchPubKey(signingRef(ring, nil)); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("emptied ring still resolves: %v", err)
	}
	if err := enc.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := enc.RemoveKey(ctx, signingRef(ring, older.Bytes)); !status.IsError(err, status.Code_Closed) {
		t.Fatalf("remove on a closed enclave = %v; want Closed", err)
	}
}
