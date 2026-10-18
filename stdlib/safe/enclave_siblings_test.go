package safe_test

// Handles opened on one tome (SD-security-sync §20.6): a mutation reloads the
// tome before it persists, so a handle opened before a sibling's install
// never drops that install; a read folds in a sibling's save, so a handle
// serves the keys a sibling installed; a removal holds across handles; and a
// reload never sheds a held record because a stale writer's tome lacks it.

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func generateSigningKey(t *testing.T, enc safe.Enclave, ringID tag.UID) safe.PubKey {
	t.Helper()
	pub, err := enc.GenerateKey(context.Background(), ringID, safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func siblingRig(t *testing.T) (safe.TomeStore, safe.Guard) {
	t.Helper()
	store := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "enclave.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte(closeTestAAD))
	t.Cleanup(func() { guard.Close() })
	return store, guard
}

func closeHandle(t *testing.T, enc safe.Enclave) {
	t.Helper()
	if err := enc.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// A handle opened before a sibling's install installs a key of its own; the
// sibling's install stays on disk beside it.
func TestEnclave_StaleHandleInstallKeepsSiblingInstall(t *testing.T) {
	store, guard := siblingRig(t)
	stale := openCloseTestEnclave(t, store, guard)
	sibling := openCloseTestEnclave(t, store, guard)
	siblingRing := tag.NewID()
	generateSigningKey(t, sibling, siblingRing)
	staleRing := tag.NewID()
	generateSigningKey(t, stale, staleRing)
	if !signingKeyOnDisk(t, store, guard, siblingRing) {
		t.Fatal("the stale handle's install dropped the sibling's durable install")
	}
	if !signingKeyOnDisk(t, store, guard, staleRing) {
		t.Fatal("the stale handle's own install is not on disk")
	}
	closeHandle(t, stale)
	closeHandle(t, sibling)
}

// A stale handle holding an install whose persist failed carries it at Close
// without dropping a sibling's durable install.
func TestEnclave_StaleUnsavedCloseKeepsSiblingInstall(t *testing.T) {
	ctx := context.Background()
	inner, guard := siblingRig(t)
	faulty := &faultTomeStore{inner: inner}
	stale := openCloseTestEnclave(t, faulty, guard)
	sibling := openCloseTestEnclave(t, inner, guard)
	siblingRing := tag.NewID()
	generateSigningKey(t, sibling, siblingRing)
	staleRing := tag.NewID()
	faulty.failSaves = 1
	if _, err := stale.GenerateKey(ctx, staleRing, safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}); err == nil {
		t.Fatal("premise void: the injected persist failure did not occur")
	}
	closeHandle(t, stale)
	if !signingKeyOnDisk(t, inner, guard, siblingRing) {
		t.Fatal("the stale handle's Close dropped the sibling's durable install")
	}
	if !signingKeyOnDisk(t, inner, guard, staleRing) {
		t.Fatal("the stale handle's Close did not carry its unsaved install")
	}
	closeHandle(t, sibling)
}

// A handle opened before a sibling's install serves that install once the
// sibling's save lands — the view a re-pointed custody entry hands out.
func TestEnclave_StaleHandleReadsSiblingInstall(t *testing.T) {
	store, guard := siblingRig(t)
	stale := openCloseTestEnclave(t, store, guard)
	sibling := openCloseTestEnclave(t, store, guard)
	ringID := tag.NewID()
	pub := generateSigningKey(t, sibling, ringID)
	ref := &safe.KeyRef{Type: safe.KeyType_SigningKey}
	ref.SetKeyringID(ringID)
	got, err := stale.FetchPubKey(ref)
	if err != nil {
		t.Fatalf("the stale handle does not serve the sibling's install: %v", err)
	}
	if string(got.Bytes) != string(pub.Bytes) {
		t.Fatal("the stale handle served a different key")
	}
	if !stale.CanSign(ref) {
		t.Fatal("the stale handle cannot sign with the sibling's install")
	}
	closeHandle(t, stale)
	closeHandle(t, sibling)
}

// A record one handle removed stays removed: a sibling that had learned it
// neither serves it nor writes it back at its next persist.
func TestEnclave_SiblingRemovalStaysRemoved(t *testing.T) {
	store, guard := siblingRig(t)
	learner := openCloseTestEnclave(t, store, guard)
	minter := openCloseTestEnclave(t, store, guard)
	ringID := tag.NewID()
	pub := generateSigningKey(t, minter, ringID)
	ref := &safe.KeyRef{
		Type:   safe.KeyType_SigningKey,
		PubKey: pub.Bytes,
	}
	ref.SetKeyringID(ringID)
	if _, err := learner.FetchPubKey(ref); err != nil {
		t.Fatalf("premise void: the learner never served the minted key: %v", err)
	}
	if err := minter.RemoveKey(context.Background(), ref); err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}
	if _, err := learner.FetchPubKey(ref); err == nil {
		t.Fatal("the learner still serves a key its sibling removed")
	}
	generateSigningKey(t, learner, tag.NewID())
	if signingKeyOnDisk(t, store, guard, ringID) {
		t.Fatal("the learner's persist wrote back a key its sibling removed")
	}
	closeHandle(t, learner)
	closeHandle(t, minter)
}

// A removal a re-install lifted still holds against a handle that synced
// before it: the remover drops a full keypair and re-imports its public half;
// a holder that still carries the private half yields to the re-installed
// record, so neither its view, its next persist nor the remover's view brings
// the private half back.
func TestEnclave_ReimportAfterRemovalKeepsPrivateHalfRemoved(t *testing.T) {
	ctx := context.Background()
	store, guard := siblingRig(t)
	holder := openCloseTestEnclave(t, store, guard)
	remover := openCloseTestEnclave(t, store, guard)
	ringID := tag.NewID()
	pub := generateSigningKey(t, holder, ringID)
	ref := &safe.KeyRef{
		Type:   safe.KeyType_SigningKey,
		PubKey: pub.Bytes,
	}
	ref.SetKeyringID(ringID)
	if !remover.CanSign(ref) {
		t.Fatal("premise void: the remover never served the holder's full keypair")
	}
	if err := remover.RemoveKey(ctx, ref); err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}
	if err := remover.ImportKey(ctx, ringID, safe.KeyPair{Pub: pub}); err != nil {
		t.Fatalf("pub-only re-import: %v", err)
	}
	generateSigningKey(t, holder, tag.NewID())
	if holder.CanSign(ref) {
		t.Error("the stale holder still signs with the removed private half")
	}
	if remover.CanSign(ref) {
		t.Error("the remover signs with the private half it removed")
	}
	reader := openCloseTestEnclave(t, store, guard)
	if reader.CanSign(ref) {
		t.Error("the stale holder's persist wrote the removed private half back")
	}
	if _, err := reader.FetchPubKey(ref); err != nil {
		t.Errorf("the re-installed public half is not on disk: %v", err)
	}
	closeHandle(t, reader)
	closeHandle(t, remover)
	closeHandle(t, holder)
}

// A stale handle cannot replace a sibling's full keypair with a pub-only
// record of the same key: the import collides with the reloaded record and
// the private half stays on disk.
func TestEnclave_StalePubOnlyImportKeepsSiblingPrivate(t *testing.T) {
	ctx := context.Background()
	store, guard := siblingRig(t)
	stale := openCloseTestEnclave(t, store, guard)
	sibling := openCloseTestEnclave(t, store, guard)
	ringID := tag.NewID()
	pub := generateSigningKey(t, sibling, ringID)
	if err := stale.ImportKey(ctx, ringID, safe.KeyPair{Pub: pub}); err == nil {
		t.Log("the pub-only import was accepted")
	}
	reader := openCloseTestEnclave(t, store, guard)
	ref := &safe.KeyRef{Type: safe.KeyType_SigningKey}
	ref.SetKeyringID(ringID)
	if !reader.CanSign(ref) {
		t.Fatal("a stale pub-only import replaced the sibling's private half on disk")
	}
	closeHandle(t, reader)
	closeHandle(t, stale)
	closeHandle(t, sibling)
}

// A writer that saves a tome lacking a held record (a stale snapshot, a
// process that does not coordinate) does not shed it: the holder's next
// persist writes it back.
func TestEnclave_PersistRestoresRecordAStaleWriterDropped(t *testing.T) {
	ctx := context.Background()
	store, guard := siblingRig(t)
	holder := openCloseTestEnclave(t, store, guard)
	snapshot, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot != nil {
		t.Fatal("premise void: the tome exists before the first install")
	}
	firstRing := tag.NewID()
	generateSigningKey(t, holder, firstRing)
	staleTome, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	secondRing := tag.NewID()
	generateSigningKey(t, holder, secondRing)
	if err := store.Save(ctx, staleTome); err != nil {
		t.Fatal(err)
	}
	if signingKeyOnDisk(t, store, guard, secondRing) {
		t.Fatal("premise void: the stale writer's tome holds the second install")
	}
	generateSigningKey(t, holder, tag.NewID())
	if !signingKeyOnDisk(t, store, guard, secondRing) {
		t.Fatal("the holder's persist shed a record the stale writer dropped")
	}
	if !signingKeyOnDisk(t, store, guard, firstRing) {
		t.Fatal("the holder's persist lost its first install")
	}
	closeHandle(t, holder)
}

// A held record and a different on-disk record of the same key reconcile to
// the one carrying the private half, whichever side holds it.
func TestEnclave_ReloadKeepsThePrivateHalf(t *testing.T) {
	ctx := context.Background()
	store, guard := siblingRig(t)
	kit, err := safe.CryptoKit(safe.Crypto.Poly25519.ID)
	if err != nil {
		t.Fatal(err)
	}
	fullPair := func() safe.KeyPair {
		pair := safe.KeyPair{
			Pub: safe.PubKey{
				CryptoKitID: kit.ID,
				KeyType:     safe.KeyType_SigningKey,
				TimeID:      tag.NowID(),
			},
		}
		if err := kit.Signing.Generate(rand.Reader, &pair); err != nil {
			t.Fatal(err)
		}
		return pair
	}
	// writeTomeHolding saves, as a writer that does not coordinate would, a
	// tome holding only pair under ringID.
	writeTomeHolding := func(ringID tag.UID, pair safe.KeyPair) {
		t.Helper()
		scratchStore := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "scratch.tome"))
		scratch := openCloseTestEnclave(t, scratchStore, guard)
		if err := scratch.ImportKey(ctx, ringID, pair); err != nil {
			t.Fatal(err)
		}
		closeHandle(t, scratch)
		sealed, err := scratchStore.Load(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Save(ctx, sealed); err != nil {
			t.Fatal(err)
		}
	}
	refFor := func(ringID tag.UID, pair safe.KeyPair) *safe.KeyRef {
		ref := &safe.KeyRef{
			Type:   safe.KeyType_SigningKey,
			PubKey: pair.Pub.Bytes,
		}
		ref.SetKeyringID(ringID)
		return ref
	}
	holder := openCloseTestEnclave(t, store, guard)

	// Held pub-only, disk full: the reload takes the disk's private half.
	adoptedRing, adopted := tag.NewID(), fullPair()
	if err := holder.ImportKey(ctx, adoptedRing, safe.KeyPair{Pub: adopted.Pub}); err != nil {
		t.Fatal(err)
	}
	writeTomeHolding(adoptedRing, adopted)
	generateSigningKey(t, holder, tag.NewID())
	if !holder.CanSign(refFor(adoptedRing, adopted)) {
		t.Fatal("the reload kept the held pub-only record over the disk's full keypair")
	}

	// Held full, disk pub-only: the holder's persist writes its private half back.
	mintedRing, minted := tag.NewID(), fullPair()
	if err := holder.ImportKey(ctx, mintedRing, minted); err != nil {
		t.Fatal(err)
	}
	writeTomeHolding(mintedRing, safe.KeyPair{Pub: minted.Pub})
	generateSigningKey(t, holder, tag.NewID())
	reader := openCloseTestEnclave(t, store, guard)
	if !reader.CanSign(refFor(mintedRing, minted)) {
		t.Fatal("a pub-only tome shed the holder's private half from disk")
	}
	if !reader.CanSign(refFor(adoptedRing, adopted)) {
		t.Fatal("the adopted key's private half is not on disk")
	}
	closeHandle(t, reader)
	closeHandle(t, holder)
}
