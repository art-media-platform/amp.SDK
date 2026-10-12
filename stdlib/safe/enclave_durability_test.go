package safe_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
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

// faultRig is an enclave over a Save-failing store, plus a reader of the tome
// on disk — the state a restart after a crash would load.
type faultRig struct {
	ctx   context.Context
	inner safe.TomeStore
	store *faultTomeStore
	guard safe.Guard
	enc   safe.Enclave
}

func newFaultRig(t *testing.T) *faultRig {
	ctx := context.Background()
	inner := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "enclave.tome"))
	guard := safe.NewFileGuard([]byte("test pass"), []byte("unsaved"))
	t.Cleanup(func() { guard.Close() })
	store := &faultTomeStore{inner: inner}
	enc, err := safe.OpenEnclave(ctx, store, guard, []byte("unsaved"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { enc.Close(ctx) })
	return &faultRig{
		ctx:   ctx,
		inner: inner,
		store: store,
		guard: guard,
		enc:   enc,
	}
}

func (rig *faultRig) onDisk(t *testing.T, ref *safe.KeyRef) bool {
	reader, err := safe.OpenEnclave(rig.ctx, rig.inner, rig.guard, []byte("unsaved"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close(rig.ctx)
	_, err = reader.FetchPubKey(ref)
	return err == nil
}

// persistOther drives one successful persist through an unrelated install.
func (rig *faultRig) persistOther(t *testing.T) {
	if err := rig.enc.ImportKey(rig.ctx, tag.NewID(), enclaveImportPair()); err != nil {
		t.Fatalf("unrelated install: %v", err)
	}
}

// A GenerateKey whose Save fails keeps its key held but unusable — no
// signature, no fetch — until a later persist puts it on disk
// (SD-security-sync §20.6 item 10; probe P6 of the V3 enclave review).
func TestEnclave_FailedGenerateUnusableUntilSaved(t *testing.T) {
	rig := newFaultRig(t)
	ringID := tag.NewID()
	ref := &safe.KeyRef{Type: safe.KeyType_SigningKey}
	ref.SetKeyringID(ringID)
	rig.store.failSaves = 1
	if _, err := rig.enc.GenerateKey(rig.ctx, ringID, safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}); err == nil {
		t.Fatal("GenerateKey reported success despite failed Save")
	}
	_, signErr := rig.enc.SignRaw(ref, []byte("digest"))
	_, fetchErr := rig.enc.FetchPubKey(ref)
	canSign := rig.enc.CanSign(ref)
	saved := rig.onDisk(t, ref)
	if canSign || signErr == nil || fetchErr == nil {
		t.Fatalf("unsaved key usable: CanSign=%v SignRaw err=%v FetchPubKey err=%v on disk=%v",
			canSign, signErr, fetchErr, saved)
	}
	if !status.IsError(fetchErr, status.Code_KeyringNotFound) {
		t.Fatalf("unsaved key must read absent (KeyringNotFound), got %v", fetchErr)
	}
	if saved {
		t.Fatal("premise void: the failed Save put the key on disk")
	}

	rig.persistOther(t)
	if !rig.enc.CanSign(ref) {
		t.Fatal("key unusable after a persist carried it to disk")
	}
	if _, err := rig.enc.SignRaw(ref, []byte("digest")); err != nil {
		t.Fatalf("SignRaw after save: %v", err)
	}
	if !rig.onDisk(t, ref) {
		t.Fatal("the successful persist did not carry the held key to disk")
	}
}

// An unsaved EncryptKey receives no wrap: its own public key does not resolve
// (an unwrap refuses), and a type-only ref — the genesis founder lookup —
// resolves the newest SAVED key of the type, never the unsaved one shadowing
// it.  After a persist the new key is the newest.
func TestEnclave_UnsavedEncryptKeyReceivesNoWrap(t *testing.T) {
	rig := newFaultRig(t)
	ringID := tag.NewID()
	saved, err := rig.enc.GenerateKey(rig.ctx, ringID, safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_AsymmetricKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	kit, err := safe.CryptoKit(safe.Crypto.Poly25519.ID)
	if err != nil {
		t.Fatal(err)
	}
	pair := safe.KeyPair{
		Pub: safe.PubKey{
			CryptoKitID: kit.ID,
			KeyType:     safe.KeyType_AsymmetricKey,
			TimeID:      tag.NowID(),
		},
	}
	if err := kit.Encrypt.Generate(rand.Reader, &pair); err != nil {
		t.Fatal(err)
	}
	rig.store.failSaves = 1
	if err := rig.enc.ImportKey(rig.ctx, ringID, pair); err == nil {
		t.Fatal("ImportKey reported success despite failed Save")
	}
	newest := &safe.KeyRef{Type: safe.KeyType_AsymmetricKey}
	newest.SetKeyringID(ringID)
	exact := &safe.KeyRef{Type: safe.KeyType_AsymmetricKey, PubKey: pair.Pub.Bytes}
	exact.SetKeyringID(ringID)
	box, err := safe.SealFor(kit.ID, pair.Pub.Bytes, []byte("planet key"))
	if err != nil {
		t.Fatal(err)
	}
	if pub, err := rig.enc.FetchPubKey(newest); err != nil || !bytes.Equal(pub.Bytes, saved.Bytes) {
		t.Fatalf("type-only ref must resolve the saved key while the newer one is unsaved: err=%v, unsaved returned: %v",
			err, err == nil && bytes.Equal(pub.Bytes, pair.Pub.Bytes))
	}
	if _, err := rig.enc.FetchPubKey(exact); err == nil {
		t.Fatal("the unsaved EncryptKey resolved by its own public key")
	}
	if _, err := rig.enc.OpenFromPub(exact, box); err == nil {
		t.Fatal("OpenFromPub opened a wrap with an unsaved key")
	}

	rig.persistOther(t)
	pub, err := rig.enc.FetchPubKey(newest)
	if err != nil || !bytes.Equal(pub.Bytes, pair.Pub.Bytes) {
		t.Fatalf("newest EncryptKey after save: err=%v", err)
	}
	if _, err := rig.enc.OpenFromPub(exact, box); err != nil {
		t.Fatalf("OpenFromPub after save: %v", err)
	}
	if !rig.onDisk(t, exact) {
		t.Fatal("the held EncryptKey is not on disk after the persist")
	}
}

// A re-install of a key whose first install failed to save supersedes the
// unsaved record — the FetchPubKey-first caller shape, whose zero TimeID
// stamps a fresh one (probe P5) — and reaches disk.
func TestEnclave_ReinstallSupersedesUnsaved(t *testing.T) {
	rig := newFaultRig(t)
	ringID := tag.NewID()
	pair := enclaveImportPair()
	pair.Pub.TimeID = tag.UID{}
	ref := &safe.KeyRef{Type: pair.Pub.KeyType}
	ref.SetKeyringID(ringID)
	rig.store.failSaves = 1
	if err := rig.enc.ImportKey(rig.ctx, ringID, pair); err == nil {
		t.Fatal("ImportKey reported success despite failed Save")
	}
	if _, err := rig.enc.FetchPubKey(ref); err == nil {
		t.Fatal("unsaved import resolved")
	}
	if err := rig.enc.ImportKey(rig.ctx, ringID, pair); err != nil {
		t.Fatalf("re-install after the key read absent: %v", err)
	}
	if _, err := rig.enc.FetchPubKey(ref); err != nil {
		t.Fatalf("re-installed key: %v", err)
	}
	if !rig.onDisk(t, ref) {
		t.Fatal("re-installed key not on disk")
	}
}
