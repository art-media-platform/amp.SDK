package safe_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// TestImportKey_NewestWinsTypeResolution pins the resolution rule the
// governance re-key follow rides (SD-member-rekey §5): importing a SECOND
// pub-only signing key under the same keyring makes it the record a type-only
// KeyRef (no PubKey prefix) resolves — the newest TimeID wins — while the
// retired record remains fetchable by explicit prefix.  VerifyChallenge
// fetches by type-only ref, so the follow's import IS the custody flip.
func TestImportKey_NewestWinsTypeResolution(t *testing.T) {
	ctx := context.Background()
	store := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "follow.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("id"))
	defer guard.Close()
	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("follow-test"))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close(ctx)

	keyringID := tag.NewID()
	keyOld := make([]byte, 32)
	rand.Read(keyOld)
	keyNew := make([]byte, 32)
	rand.Read(keyNew)

	importPubOnly := func(pub []byte, at time.Time) {
		t.Helper()
		if err := enc.ImportKey(ctx, keyringID, safe.KeyPair{
			Pub: safe.PubKey{
				CryptoKitID: safe.Crypto.Poly25519.ID,
				KeyType:     safe.KeyType_SigningKey,
				TimeID:      tag.UID_FromTime(at),
				Bytes:       pub,
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	importPubOnly(keyOld, time.Now().Add(-time.Hour))
	importPubOnly(keyNew, time.Now())

	typeRef := &safe.KeyRef{Type: safe.KeyType_SigningKey}
	typeRef.SetKeyringID(keyringID)
	got, err := enc.FetchPubKey(typeRef)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes, keyNew) {
		t.Error("a type-only ref must resolve the NEWEST signing record after the follow import")
	}

	prefixRef := &safe.KeyRef{Type: safe.KeyType_SigningKey, PubKey: keyOld}
	prefixRef.SetKeyringID(keyringID)
	if prior, err := enc.FetchPubKey(prefixRef); err != nil || !bytes.Equal(prior.Bytes, keyOld) {
		t.Error("the retired record must remain fetchable by explicit prefix")
	}
}
