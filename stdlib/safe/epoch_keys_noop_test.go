package safe

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// countingTomeStore counts a local tome's loads and saves and can fail the
// next failSaves saves.
type countingTomeStore struct {
	inner     TomeStore
	path      string
	loads     atomic.Int64
	saves     atomic.Int64
	failSaves atomic.Int64
}

func newCountingTomeStore(t *testing.T) *countingTomeStore {
	path := filepath.Join(t.TempDir(), "epoch-keys.tome")
	return &countingTomeStore{
		inner: NewLocalTomeStore(path),
		path:  path,
	}
}

func (store *countingTomeStore) TomeStoreIdentity() string {
	return store.inner.(TomeStoreIdentity).TomeStoreIdentity()
}

func (store *countingTomeStore) Load(ctx context.Context) (*SealedTome, error) {
	store.loads.Add(1)
	return store.inner.Load(ctx)
}

func (store *countingTomeStore) Save(ctx context.Context, sealed *SealedTome) error {
	if store.failSaves.Load() > 0 {
		store.failSaves.Add(-1)
		return errors.New("countingTomeStore: injected Save failure")
	}
	store.saves.Add(1)
	return store.inner.Save(ctx, sealed)
}

// noopBed is two handles on one epoch tome: writer puts, sibling reads.
type noopBed struct {
	t       *testing.T
	ctx     context.Context
	store   *countingTomeStore
	guard   Guard
	writer  *epochKeyStore
	sibling *epochKeyStore
}

func newNoopBed(t *testing.T) *noopBed {
	t.Helper()
	bed := &noopBed{
		t:     t,
		ctx:   context.Background(),
		store: newCountingTomeStore(t),
		guard: NewFileGuard([]byte("pass"), []byte("put-noop")),
	}
	t.Cleanup(func() { bed.guard.Close() })
	bed.writer = bed.open()
	bed.sibling = bed.open()
	return bed
}

func (bed *noopBed) open() *epochKeyStore {
	bed.t.Helper()
	eks, err := OpenEpochKeyStore(bed.ctx, bed.store, bed.guard, []byte("put-noop"))
	if err != nil {
		bed.t.Fatalf("OpenEpochKeyStore: %v", err)
	}
	bed.t.Cleanup(func() { eks.Close(bed.ctx) })
	return eks.(*epochKeyStore)
}

func (bed *noopBed) tomeBytes() []byte {
	bed.t.Helper()
	raw, err := os.ReadFile(bed.store.path)
	if err != nil {
		bed.t.Fatal(err)
	}
	return raw
}

// syncSibling has the sibling read, so its view is at the current revision.
func (bed *noopBed) syncSibling(scope KeyScope) {
	bed.t.Helper()
	if _, err := bed.sibling.HeldEpochs(scope); err != nil {
		bed.t.Fatal(err)
	}
}

// siblingReloads reports whether the sibling's next read loads the tome.
func (bed *noopBed) siblingReloads(scope KeyScope) bool {
	bed.t.Helper()
	before := bed.store.loads.Load()
	bed.syncSibling(scope)
	return bed.store.loads.Load() != before
}

// tomeMark is the persisted tome and its coordination counters at one moment.
type tomeMark struct {
	raw      []byte
	saves    int64
	revision uint64
}

func (bed *noopBed) mark() tomeMark {
	return tomeMark{
		raw:      bed.tomeBytes(),
		saves:    bed.store.saves.Load(),
		revision: bed.writer.shared.revision.Load(),
	}
}

func (bed *noopBed) requireUnchanged(label string, before tomeMark, scope KeyScope) {
	bed.t.Helper()
	after := bed.mark()
	if !bytes.Equal(after.raw, before.raw) || after.saves != before.saves || after.revision != before.revision {
		bed.t.Errorf("%s: tome rewritten (bytes equal %v, saves %d→%d, revision %d→%d)",
			label, bytes.Equal(after.raw, before.raw), before.saves, after.saves, before.revision, after.revision)
	}
	if bed.siblingReloads(scope) {
		bed.t.Errorf("%s: the sibling reloaded after a put that changed nothing", label)
	}
}

func (bed *noopBed) requireSaved(label string, before tomeMark, scope KeyScope) {
	bed.t.Helper()
	after := bed.mark()
	if bytes.Equal(after.raw, before.raw) || after.saves != before.saves+1 || after.revision == before.revision {
		bed.t.Errorf("%s: no save (bytes equal %v, saves %d→%d, revision %d→%d)",
			label, bytes.Equal(after.raw, before.raw), before.saves, after.saves, before.revision, after.revision)
	}
	if !bed.siblingReloads(scope) {
		bed.t.Errorf("%s: the sibling did not reload after a save", label)
	}
}

func noopKeyBytes(t *testing.T) []byte {
	t.Helper()
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatal(err)
	}
	return keyBytes
}

func noopKey(epochID tag.UID, role KeyRole, keyBytes []byte) SymKey {
	return SymKey{
		CryptoKitID: Crypto.Poly25519.ID,
		EpochID:     epochID,
		Role:        role,
		Bytes:       append([]byte(nil), keyBytes...),
	}
}

// TestEpochKeys_NoChangePutWritesNothing: a PutKey or StageKey of held,
// identical material, with the election as it stands, leaves the tome's
// bytes, its save count and the shared revision as they were, and the
// sibling does not reload.
func TestEpochKeys_NoChangePutWritesNothing(t *testing.T) {
	bed := newNoopBed(t)
	scope := Scope(tag.NewID())
	olderEpoch := tag.UID{100, 1}
	newerEpoch := tag.UID{200, 2}
	olderBytes := noopKeyBytes(t)
	newerBytes := noopKeyBytes(t)
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(olderEpoch, KeyRole_ContentKey, olderBytes)); err != nil {
		t.Fatal(err)
	}
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(newerEpoch, KeyRole_ContentKey, newerBytes)); err != nil {
		t.Fatal(err)
	}
	bed.syncSibling(scope)

	puts := []struct {
		label string
		put   func(SymKey) error
	}{
		{"PutKey", func(key SymKey) error { return bed.writer.PutKey(bed.ctx, scope, key) }},
		{"StageKey", func(key SymKey) error { return bed.writer.StageKey(bed.ctx, scope, key) }},
	}
	for _, held := range []struct {
		label    string
		epochID  tag.UID
		keyBytes []byte
	}{
		{"elected epoch", newerEpoch, newerBytes},
		{"older epoch", olderEpoch, olderBytes},
	} {
		for _, mode := range puts {
			label := mode.label + " " + held.label
			before := bed.mark()
			if err := mode.put(noopKey(held.epochID, KeyRole_ContentKey, held.keyBytes)); err != nil {
				t.Fatalf("%s: %v", label, err)
			}
			bed.requireUnchanged(label, before, scope)
		}
	}
	current, err := bed.sibling.GetCurrentKey(scope, KeyRole_ContentKey)
	if err != nil || current.EpochID != newerEpoch || !bytes.Equal(current.Bytes, newerBytes) {
		t.Errorf("current %s (%v), want the newer epoch's held key", current.EpochID.Base32(), err)
	}
	current.Zero()
}

// unnamedTomeStore forwards a local tome but names no TomeStoreIdentity, so
// handles over two of them on one file do not coordinate.
type unnamedTomeStore struct {
	inner TomeStore
}

func (store *unnamedTomeStore) Load(ctx context.Context) (*SealedTome, error) {
	return store.inner.Load(ctx)
}

func (store *unnamedTomeStore) Save(ctx context.Context, sealed *SealedTome) error {
	return store.inner.Save(ctx, sealed)
}

// TestEpochKeys_NoChangePutRefreshesTheHandle: a handle that another,
// uncoordinated writer got ahead of serves the key its own no-change PutKey
// found on disk.
func TestEpochKeys_NoChangePutRefreshesTheHandle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "epoch-keys.tome")
	guard := NewFileGuard([]byte("pass"), []byte("put-noop"))
	t.Cleanup(func() { guard.Close() })
	open := func() EpochKeyStore {
		eks, err := OpenEpochKeyStore(ctx, &unnamedTomeStore{inner: NewLocalTomeStore(path)}, guard, []byte("put-noop"))
		if err != nil {
			t.Fatalf("OpenEpochKeyStore: %v", err)
		}
		t.Cleanup(func() { eks.Close(ctx) })
		return eks
	}
	handleA, handleB := open(), open()
	scope := Scope(tag.NewID())
	epochID := tag.UID{100, 1}
	keyBytes := noopKeyBytes(t)
	if err := handleB.PutKey(ctx, scope, noopKey(epochID, KeyRole_ContentKey, keyBytes)); err != nil {
		t.Fatal(err)
	}
	if err := handleA.PutKey(ctx, scope, noopKey(epochID, KeyRole_ContentKey, keyBytes)); err != nil {
		t.Fatalf("equal PutKey: %v", err)
	}
	held, err := handleA.GetKey(scope, epochID, KeyRole_ContentKey)
	if err != nil || !bytes.Equal(held.Bytes, keyBytes) {
		t.Errorf("GetKey after the equal PutKey: %v", err)
	}
	held.Zero()
	current, err := handleA.GetCurrentKey(scope, KeyRole_ContentKey)
	if err != nil || current.EpochID != epochID || !bytes.Equal(current.Bytes, keyBytes) {
		t.Errorf("GetCurrentKey after the equal PutKey: %s (%v)", current.EpochID.Base32(), err)
	}
	current.Zero()
}

// TestEpochKeys_EqualInstallStillSaves: an InstallKey of held, identical
// material saves, moves the revision and reloads the sibling, reports nothing
// installed, and surfaces a failing Save.
func TestEpochKeys_EqualInstallStillSaves(t *testing.T) {
	bed := newNoopBed(t)
	scope := Scope(tag.NewID())
	epochID := tag.UID{100, 1}
	heldBytes := noopKeyBytes(t)
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(epochID, KeyRole_ContentKey, heldBytes)); err != nil {
		t.Fatal(err)
	}
	bed.syncSibling(scope)

	before := bed.mark()
	installed, err := bed.writer.InstallKey(bed.ctx, scope, noopKey(epochID, KeyRole_ContentKey, heldBytes))
	if err != nil || installed {
		t.Fatalf("equal InstallKey: installed=%v err=%v", installed, err)
	}
	bed.requireSaved("equal InstallKey", before, scope)

	bed.store.failSaves.Store(1)
	if _, err := bed.writer.InstallKey(bed.ctx, scope, noopKey(epochID, KeyRole_ContentKey, heldBytes)); err == nil {
		t.Error("equal InstallKey hid a failing Save")
	}
	if bed.store.failSaves.Load() != 0 {
		t.Error("equal InstallKey did not attempt a Save")
	}
}

// TestEpochKeys_ChangingPutStillSaves: every put that changes what the tome
// holds saves, moves the revision and reloads the sibling; the refusals stay
// refusals and save nothing.
func TestEpochKeys_ChangingPutStillSaves(t *testing.T) {
	bed := newNoopBed(t)
	scope := Scope(tag.NewID())
	electedEpoch := tag.UID{100, 1}
	stagedEpoch := tag.UID{200, 2}
	newestEpoch := tag.UID{300, 3}
	heldBytes := noopKeyBytes(t)
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(electedEpoch, KeyRole_ContentKey, heldBytes)); err != nil {
		t.Fatal(err)
	}
	bed.syncSibling(scope)

	before := bed.mark()
	replaced := noopKeyBytes(t)
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(electedEpoch, KeyRole_ContentKey, replaced)); err != nil {
		t.Fatal(err)
	}
	bed.requireSaved("new material", before, scope)

	before = bed.mark()
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(electedEpoch, KeyRole_WriteSeed, noopKeyBytes(t))); err != nil {
		t.Fatal(err)
	}
	bed.requireSaved("new role", before, scope)

	before = bed.mark()
	conflict := noopKey(electedEpoch, KeyRole_ContentKey, replaced)
	conflict.CryptoKitID = tag.UID{7, 7}
	if err := bed.writer.PutKey(bed.ctx, scope, conflict); !status.IsError(err, status.Code_AuthFailed) {
		t.Errorf("kit conflict: %v, want AuthFailed", err)
	}
	if _, err := bed.writer.InstallKey(bed.ctx, scope, noopKey(electedEpoch, KeyRole_ContentKey, noopKeyBytes(t))); !status.IsError(err, status.Code_AuthFailed) {
		t.Errorf("material conflict: %v, want AuthFailed", err)
	}
	bed.requireUnchanged("refusals", before, scope)

	stagedBytes := noopKeyBytes(t)
	before = bed.mark()
	if err := bed.writer.StageKey(bed.ctx, scope, noopKey(stagedEpoch, KeyRole_ContentKey, stagedBytes)); err != nil {
		t.Fatal(err)
	}
	bed.requireSaved("stage", before, scope)

	before = bed.mark()
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(stagedEpoch, KeyRole_ContentKey, stagedBytes)); err != nil {
		t.Fatal(err)
	}
	bed.requireSaved("staged key then a plain put (the election moves)", before, scope)
	current, err := bed.open().GetCurrentKey(scope, KeyRole_ContentKey)
	if err != nil || current.EpochID != stagedEpoch {
		t.Errorf("reopened current %s (%v), want the staged epoch elected", current.EpochID.Base32(), err)
	}
	current.Zero()

	before = bed.mark()
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(newestEpoch, KeyRole_ContentKey, noopKeyBytes(t))); err != nil {
		t.Fatal(err)
	}
	bed.requireSaved("newer epoch", before, scope)
}

// TestEpochKeys_NoChangePutCarriesPendingDestruction: while a failed shred's
// destruction is pending, a put that changes nothing else still saves, so
// the destruction reaches the tome.
func TestEpochKeys_NoChangePutCarriesPendingDestruction(t *testing.T) {
	bed := newNoopBed(t)
	scope := Scope(tag.NewID())
	cutEpoch := tag.UID{100, 1}
	keptEpoch := tag.UID{200, 2}
	keptBytes := noopKeyBytes(t)
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(cutEpoch, KeyRole_ContentKey, noopKeyBytes(t))); err != nil {
		t.Fatal(err)
	}
	if err := bed.writer.PutKey(bed.ctx, scope, noopKey(keptEpoch, KeyRole_ContentKey, keptBytes)); err != nil {
		t.Fatal(err)
	}
	bed.store.failSaves.Store(1)
	if err := bed.writer.ShredKeys(bed.ctx, scope, []tag.UID{cutEpoch}); err == nil {
		t.Fatal("ShredKeys with a failing Save must error")
	}
	bed.syncSibling(scope)

	before := bed.mark()
	if err := bed.sibling.PutKey(bed.ctx, scope, noopKey(keptEpoch, KeyRole_ContentKey, keptBytes)); err != nil {
		t.Fatal(err)
	}
	if after := bed.mark(); after.saves != before.saves+1 || bytes.Equal(after.raw, before.raw) {
		t.Errorf("the put during a pending destruction did not save (saves %d→%d)", before.saves, after.saves)
	}
	fresh := bed.open()
	if _, err := fresh.GetKey(scope, cutEpoch, KeyRole_ContentKey); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Errorf("the pending destruction did not reach the tome: %v", err)
	}
	kept, err := fresh.GetKey(scope, keptEpoch, KeyRole_ContentKey)
	if err != nil || !bytes.Equal(kept.Bytes, keptBytes) {
		t.Errorf("the kept key did not round-trip: %v", err)
	}
	kept.Zero()
}
