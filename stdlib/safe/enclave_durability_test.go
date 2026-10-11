package safe_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func enclaveImportPair() safe.KeyPair {
	return safe.KeyPair{
		Pub: safe.PubKey{
			CryptoKitID: safe.Crypto.Poly25519.ID,
			KeyType:     safe.KeyType_SymmetricKey,
			TimeID:      tag.NowID(),
			Bytes:       bytes.Repeat([]byte{0x51}, 32),
		},
		Prv: bytes.Repeat([]byte{0x64}, 32),
	}
}

// An import whose persist fails stays held in memory; an exact duplicate of it
// must retry the persist, report each failure, and succeed only once the tome
// on disk holds the key.
func TestEnclave_ImportFailedPersistRetryDurable(t *testing.T) {
	for _, failures := range []int{1, 2} {
		t.Run(fmt.Sprintf("Save/%d", failures), func(t *testing.T) {
			ctx := context.Background()
			inner := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "enclave.tome"))
			store := &faultTomeStore{inner: inner}
			guard := safe.NewFileGuard([]byte("test pass"), []byte("import retry"))
			defer guard.Close()
			enc, err := safe.OpenEnclave(ctx, store, guard, []byte("import retry"))
			if err != nil {
				t.Fatal(err)
			}
			defer enc.Close(ctx)
			ringID := tag.NewID()
			pair := enclaveImportPair()
			store.failSaves = failures
			for attempt := 0; attempt < failures; attempt++ {
				if err := enc.ImportKey(ctx, ringID, pair); err == nil {
					if attempt == 0 {
						t.Fatal("initial injected persistence failure did not occur")
					}
					t.Errorf("attempt %d reported success while persistence still fails", attempt+1)
				}
			}
			if err := enc.ImportKey(ctx, ringID, pair); err != nil {
				t.Fatalf("retry after persistence recovers: %v", err)
			}

			// A fresh reader opens the actual file without closing the writer first.
			reopened, err := safe.OpenEnclave(ctx, inner, guard, []byte("import retry"))
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close(ctx)
			ref := &safe.KeyRef{Type: safe.KeyType_SymmetricKey}
			ref.SetKeyringID(ringID)
			key, err := reopened.ExportSymmetricKey(ref)
			if err != nil {
				t.Fatalf("successful retry did not persist the key: %v", err)
			}
			safe.Zero(key)
		})
	}
}

// A GenerateKey whose Save fails leaves its key held; the next exact duplicate
// import must carry that key to disk before it reports success.
func TestEnclave_DuplicateCarriesFailedGenerationToDisk(t *testing.T) {
	ctx := context.Background()
	inner := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "enclave.tome"))
	store := &faultTomeStore{inner: inner}
	guard := safe.NewFileGuard([]byte("test pass"), []byte("generate retry"))
	defer guard.Close()
	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("generate retry"))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close(ctx)
	baselineRing := tag.NewID()
	pair := enclaveImportPair()
	if err := enc.ImportKey(ctx, baselineRing, pair); err != nil {
		t.Fatal(err)
	}
	generatedRing := tag.NewID()
	store.failSaves = 1
	if _, err := enc.GenerateKey(ctx, generatedRing, safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}); err == nil {
		t.Fatal("GenerateKey reported success despite failed Save")
	}
	if err := enc.ImportKey(ctx, baselineRing, pair); err != nil {
		t.Fatalf("duplicate after failed generation: %v", err)
	}
	reopened, err := safe.OpenEnclave(ctx, inner, guard, []byte("generate retry"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(ctx)
	ref := &safe.KeyRef{Type: safe.KeyType_SigningKey}
	ref.SetKeyringID(generatedRing)
	if !reopened.CanSign(ref) {
		t.Fatal("duplicate reported success while generated private key remained unsaved")
	}
}
