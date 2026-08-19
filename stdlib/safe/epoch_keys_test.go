package safe_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func randomKeyBytes(t *testing.T) []byte {
	t.Helper()
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatal(err)
	}
	return keyBytes
}

func putEpochKey(t *testing.T, eks safe.EpochKeyStore, containerID, epochID tag.UID, role safe.KeyRole, keyBytes []byte) {
	t.Helper()
	err := eks.PutKey(context.Background(), containerID, safe.SymKey{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		EpochID:     epochID,
		Role:        role,
		Bytes:       keyBytes,
	})
	if err != nil {
		t.Fatalf("PutKey(%s role=%v): %v", epochID.Base32(), role, err)
	}
}

// TestEpochKeys_ShredDurableAtReturn pins the ShredKeys durability contract: the
// removal persists BEFORE the method returns, so a crash after return (modeled
// by reopening the tome WITHOUT Close) cannot resurrect a shredded key — a
// shred that can un-shred is not a shred.  Survivor identity is asserted as
// BYTES (golden-fixture doctrine).
func TestEpochKeys_ShredDurableAtReturn(t *testing.T) {
	ctx := context.Background()
	store := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "epoch-keys.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("shred-test"))
	defer guard.Close()

	eks, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("shred-test"))
	if err != nil {
		t.Fatalf("OpenEpochKeyStore: %v", err)
	}

	containerID := tag.NewID()
	cutEpoch := tag.NewID()
	liveEpoch := tag.NewID()
	cutContent := randomKeyBytes(t)
	cutWriteSeed := randomKeyBytes(t)
	liveContent := randomKeyBytes(t)

	putEpochKey(t, eks, containerID, cutEpoch, safe.KeyRole_ContentKey, cutContent)
	putEpochKey(t, eks, containerID, cutEpoch, safe.KeyRole_WriteSeed, cutWriteSeed)
	putEpochKey(t, eks, containerID, liveEpoch, safe.KeyRole_ContentKey, liveContent)

	if err := eks.ShredKeys(ctx, []tag.UID{cutEpoch}); err != nil {
		t.Fatalf("ShredKeys: %v", err)
	}

	// Live session: every role of the cut epoch is gone, reading as key-absent
	// (KeyringNotFound), never as a store-lifecycle error.
	for _, role := range []safe.KeyRole{safe.KeyRole_ContentKey, safe.KeyRole_WriteSeed} {
		if _, err := eks.GetKey(containerID, cutEpoch, role); !status.IsError(err, status.Code_KeyringNotFound) {
			t.Fatalf("shredded epoch role=%v: got %v, want Code_KeyringNotFound", role, err)
		}
	}
	liveKey, err := eks.GetKey(containerID, liveEpoch, safe.KeyRole_ContentKey)
	if err != nil {
		t.Fatalf("live epoch after shred: %v", err)
	}
	if !bytes.Equal(liveKey.Bytes, liveContent) {
		t.Fatal("live epoch key bytes changed across a shred of a different epoch")
	}
	liveKey.Zero()

	// Idempotent: a second shred of the same set is a no-op success.
	if err := eks.ShredKeys(ctx, []tag.UID{cutEpoch}); err != nil {
		t.Fatalf("ShredKeys re-run: %v", err)
	}

	// Crash model: NO Close — reopen straight from the persisted tome.  The cut
	// epoch must be gone and the live epoch must round-trip byte-identical.
	reopened, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("shred-test"))
	if err != nil {
		t.Fatalf("reopen without Close: %v", err)
	}
	defer reopened.Close(ctx)

	if _, err := reopened.GetKey(containerID, cutEpoch, safe.KeyRole_ContentKey); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("shredded key resurrected across reopen: %v", err)
	}
	survivor, err := reopened.GetKey(containerID, liveEpoch, safe.KeyRole_ContentKey)
	if err != nil {
		t.Fatalf("live epoch after reopen: %v", err)
	}
	if !bytes.Equal(survivor.Bytes, liveContent) {
		t.Fatal("live epoch key bytes did not round-trip the shred-persisted tome")
	}
	survivor.Zero()
}

// TestEpochKeys_PutDurableAtReturn pins the install durability contract: an
// installed epoch key persists BEFORE PutKey returns, so a crash after return
// (modeled by reopening the tome WITHOUT Close) cannot lose it.  A key that
// exists only until clean Close is a key an unclean kill deletes — the
// founder-loses-its-confidential-planet defect (O4 §4.12 caution).
func TestEpochKeys_PutDurableAtReturn(t *testing.T) {
	ctx := context.Background()
	store := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "epoch-keys.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("put-durable"))
	defer guard.Close()

	eks, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("put-durable"))
	if err != nil {
		t.Fatalf("OpenEpochKeyStore: %v", err)
	}

	containerID := tag.NewID()
	epochID := tag.NowID()
	contentKey := randomKeyBytes(t)
	writeSeed := randomKeyBytes(t)
	putEpochKey(t, eks, containerID, epochID, safe.KeyRole_ContentKey, contentKey)
	putEpochKey(t, eks, containerID, epochID, safe.KeyRole_WriteSeed, writeSeed)

	// Crash model: NO Close — reopen straight from the persisted tome.  Every
	// installed role must round-trip byte-identical, and the container's
	// current-epoch election must re-derive to the installed epoch.
	reopened, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("put-durable"))
	if err != nil {
		t.Fatalf("reopen without Close: %v", err)
	}
	defer reopened.Close(ctx)

	for _, probe := range []struct {
		role safe.KeyRole
		want []byte
	}{
		{safe.KeyRole_ContentKey, contentKey},
		{safe.KeyRole_WriteSeed, writeSeed},
	} {
		got, err := reopened.GetKey(containerID, epochID, probe.role)
		if err != nil {
			t.Fatalf("role=%v lost across a no-Close reopen — PutKey is not durable at return: %v", probe.role, err)
		}
		if !bytes.Equal(got.Bytes, probe.want) {
			t.Fatalf("role=%v bytes did not round-trip the install persist", probe.role)
		}
		got.Zero()
	}
	current, err := reopened.GetCurrentKey(containerID, safe.KeyRole_ContentKey)
	if err != nil {
		t.Fatalf("current epoch not re-derived after no-Close reopen: %v", err)
	}
	if current.EpochID != epochID {
		t.Fatalf("current epoch %s, want installed %s", current.EpochID.Base32(), epochID.Base32())
	}
	current.Zero()

	if err := eks.Close(ctx); err != nil {
		t.Fatalf("Close (installing session): %v", err)
	}
}

// TestEpochKeys_SetCurrentDurableAtReturn pins the election durability
// contract: an explicit SetCurrentEpoch of an OLDER epoch persists BEFORE the
// method returns, so a crash after return (modeled by reopening the tome
// WITHOUT Close) cannot regress the current pointer to the
// newest-per-container fallback.
func TestEpochKeys_SetCurrentDurableAtReturn(t *testing.T) {
	ctx := context.Background()
	store := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "epoch-keys.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("set-current"))
	defer guard.Close()

	eks, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("set-current"))
	if err != nil {
		t.Fatalf("OpenEpochKeyStore: %v", err)
	}

	containerID := tag.NewID()
	olderEpoch := tag.UID{100, 1} // EpochID is time-based: [0] then [1] orders it
	newerEpoch := tag.UID{200, 2}
	olderContent := randomKeyBytes(t)
	putEpochKey(t, eks, containerID, olderEpoch, safe.KeyRole_ContentKey, olderContent)
	putEpochKey(t, eks, containerID, newerEpoch, safe.KeyRole_ContentKey, randomKeyBytes(t))

	// Precondition: PutKey auto-elected the newer epoch.
	current, err := eks.GetCurrentKey(containerID, safe.KeyRole_ContentKey)
	if err != nil {
		t.Fatalf("GetCurrentKey precondition: %v", err)
	}
	if current.EpochID != newerEpoch {
		t.Fatalf("precondition: current %s, want auto-elected newer %s", current.EpochID.Base32(), newerEpoch.Base32())
	}
	current.Zero()

	if err := eks.SetCurrentEpoch(ctx, containerID, olderEpoch); err != nil {
		t.Fatalf("SetCurrentEpoch(older): %v", err)
	}

	// Crash model: NO Close — reopen straight from the persisted tome.  The
	// explicit election must survive; a recompute would regress to newerEpoch.
	reopened, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("set-current"))
	if err != nil {
		t.Fatalf("reopen without Close: %v", err)
	}
	defer reopened.Close(ctx)

	elected, err := reopened.GetCurrentKey(containerID, safe.KeyRole_ContentKey)
	if err != nil {
		t.Fatalf("GetCurrentKey after reopen: %v", err)
	}
	if elected.EpochID != olderEpoch {
		t.Fatalf("election regressed across a no-Close reopen: current %s, want elected %s — SetCurrentEpoch is not durable at return",
			elected.EpochID.Base32(), olderEpoch.Base32())
	}
	if !bytes.Equal(elected.Bytes, olderContent) {
		t.Fatal("elected epoch key bytes did not round-trip the election persist")
	}
	elected.Zero()

	if err := eks.Close(ctx); err != nil {
		t.Fatalf("Close (electing session): %v", err)
	}
}

// faultTomeStore delegates to a real TomeStore but fails the next failSaves
// Save calls — the fault seam for the shred durability contract.
type faultTomeStore struct {
	inner     safe.TomeStore
	failSaves int
}

func (fs *faultTomeStore) Load(ctx context.Context) (*safe.SealedTome, error) {
	return fs.inner.Load(ctx)
}

func (fs *faultTomeStore) Save(ctx context.Context, sealed *safe.SealedTome) error {
	if fs.failSaves > 0 {
		fs.failSaves--
		return errors.New("faultTomeStore: injected Save failure")
	}
	return fs.inner.Save(ctx, sealed)
}

// TestEpochKeys_ShredFailedSaveRetryReachesDisk pins the failed-persist half of
// the shred contract: a shred whose Save fails ERRORS, and the same-session
// retry — which finds the keys already gone from memory — must still reach
// disk before reporting success.  A retry that returns nil off memory state
// alone leaves the key on disk, and Close (no longer dirty) would seal that
// in: the resurrected-key hole the shred-marks-dirty flag closes.
func TestEpochKeys_ShredFailedSaveRetryReachesDisk(t *testing.T) {
	ctx := context.Background()
	inner := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "epoch-keys.tome"))
	store := &faultTomeStore{inner: inner}
	guard := safe.NewFileGuard([]byte("pass"), []byte("shred-fault"))
	defer guard.Close()

	eks, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("shred-fault"))
	if err != nil {
		t.Fatalf("OpenEpochKeyStore: %v", err)
	}
	containerID := tag.NewID()
	cutEpoch := tag.NewID()
	liveEpoch := tag.NewID()
	liveContent := randomKeyBytes(t)
	putEpochKey(t, eks, containerID, cutEpoch, safe.KeyRole_ContentKey, randomKeyBytes(t))
	putEpochKey(t, eks, containerID, liveEpoch, safe.KeyRole_ContentKey, liveContent)
	if err := eks.Close(ctx); err != nil {
		t.Fatalf("Close (seed the tome): %v", err)
	}

	// Reopen on the same tome; arm ONE Save failure under the shred.
	eks, err = safe.OpenEpochKeyStore(ctx, store, guard, []byte("shred-fault"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	store.failSaves = 1
	if err := eks.ShredKeys(ctx, []tag.UID{cutEpoch}); err == nil {
		t.Fatal("ShredKeys with a failing Save must error — durable-at-return means the failure surfaces")
	}

	// The retry finds nothing left in memory but the removal is still unsaved:
	// it must persist (and succeed) before returning nil.
	if err := eks.ShredKeys(ctx, []tag.UID{cutEpoch}); err != nil {
		t.Fatalf("ShredKeys retry after failed Save: %v", err)
	}
	reopened, err := safe.OpenEpochKeyStore(ctx, inner, guard, []byte("shred-fault"))
	if err != nil {
		t.Fatalf("reopen after retry: %v", err)
	}
	if _, err := reopened.GetKey(containerID, cutEpoch, safe.KeyRole_ContentKey); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("shredded key survived on disk after the retry reported success: %v", err)
	}
	survivor, err := reopened.GetKey(containerID, liveEpoch, safe.KeyRole_ContentKey)
	if err != nil {
		t.Fatalf("live key after retry: %v", err)
	}
	if !bytes.Equal(survivor.Bytes, liveContent) {
		t.Fatal("live key bytes did not round-trip the retry persist")
	}
	survivor.Zero()
	if err := reopened.Close(ctx); err != nil {
		t.Fatalf("Close (reopened): %v", err)
	}
	if err := eks.Close(ctx); err != nil {
		t.Fatalf("Close (shredding session): %v", err)
	}
}

// TestEpochKeys_ShredFailedSaveCloseReachesDisk is the Close-path twin: no
// retry — the session just closes after the failed shred.  The shred marked
// the store dirty, so Close's persist carries the removal to disk.
func TestEpochKeys_ShredFailedSaveCloseReachesDisk(t *testing.T) {
	ctx := context.Background()
	inner := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "epoch-keys.tome"))
	store := &faultTomeStore{inner: inner}
	guard := safe.NewFileGuard([]byte("pass"), []byte("shred-fault-close"))
	defer guard.Close()

	eks, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("shred-fault-close"))
	if err != nil {
		t.Fatalf("OpenEpochKeyStore: %v", err)
	}
	containerID := tag.NewID()
	cutEpoch := tag.NewID()
	putEpochKey(t, eks, containerID, cutEpoch, safe.KeyRole_ContentKey, randomKeyBytes(t))
	if err := eks.Close(ctx); err != nil {
		t.Fatalf("Close (seed the tome): %v", err)
	}

	eks, err = safe.OpenEpochKeyStore(ctx, store, guard, []byte("shred-fault-close"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	store.failSaves = 1
	if err := eks.ShredKeys(ctx, []tag.UID{cutEpoch}); err == nil {
		t.Fatal("ShredKeys with a failing Save must error")
	}
	if err := eks.Close(ctx); err != nil {
		t.Fatalf("Close after failed shred: %v", err)
	}
	reopened, err := safe.OpenEpochKeyStore(ctx, inner, guard, []byte("shred-fault-close"))
	if err != nil {
		t.Fatalf("reopen after Close: %v", err)
	}
	defer reopened.Close(ctx)
	if _, err := reopened.GetKey(containerID, cutEpoch, safe.KeyRole_ContentKey); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("shredded key survived a Close after the failed shred persist: %v", err)
	}
}

// TestEpochKeys_ShredCurrentEpochFailsClosed pins the dangling-current posture:
// shredding a container's current epoch leaves it with NO current epoch (no
// silent re-election) — GetCurrentKey reads key-absent until PutKey or
// SetCurrentEpoch names a successor.
func TestEpochKeys_ShredCurrentEpochFailsClosed(t *testing.T) {
	ctx := context.Background()
	store := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "epoch-keys.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("shred-current"))
	defer guard.Close()

	eks, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("shred-current"))
	if err != nil {
		t.Fatalf("OpenEpochKeyStore: %v", err)
	}
	defer eks.Close(ctx)

	containerID := tag.NewID()
	epochA := tag.NewID()
	epochB := tag.NewID()
	putEpochKey(t, eks, containerID, epochA, safe.KeyRole_ContentKey, randomKeyBytes(t))
	putEpochKey(t, eks, containerID, epochB, safe.KeyRole_ContentKey, randomKeyBytes(t))

	// Learn which epoch the store elected current, then shred exactly it.
	elected, err := eks.GetCurrentKey(containerID, safe.KeyRole_ContentKey)
	if err != nil {
		t.Fatalf("GetCurrentKey precondition: %v", err)
	}
	currentEpoch := elected.EpochID
	elected.Zero()
	successor := epochA
	if currentEpoch == epochA {
		successor = epochB
	}

	if err := eks.ShredKeys(ctx, []tag.UID{currentEpoch}); err != nil {
		t.Fatalf("ShredKeys(current): %v", err)
	}
	if _, err := eks.GetCurrentKey(containerID, safe.KeyRole_ContentKey); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("current pointer at a shredded epoch must fail closed, got %v", err)
	}
	if err := eks.SetCurrentEpoch(ctx, containerID, successor); err != nil {
		t.Fatalf("SetCurrentEpoch(successor): %v", err)
	}
	if _, err := eks.GetCurrentKey(containerID, safe.KeyRole_ContentKey); err != nil {
		t.Fatalf("named successor must serve as current: %v", err)
	}
}

// TestEpochKeys_ClosedStoreSentinel pins the closed-store contract: every
// EpochKeyStore method on a closed store returns the typed safe.ErrStoreClosed,
// classifiable via errors.Is — the identity planet-side consumers use to read a
// logout racing a decrypt as no-custody (retryable), never forgery (final).
func TestEpochKeys_ClosedStoreSentinel(t *testing.T) {
	ctx := context.Background()
	store := safe.NewLocalTomeStore(filepath.Join(t.TempDir(), "epoch-keys.tome"))
	guard := safe.NewFileGuard([]byte("pass"), []byte("closed-test"))
	defer guard.Close()

	eks, err := safe.OpenEpochKeyStore(ctx, store, guard, []byte("closed-test"))
	if err != nil {
		t.Fatalf("OpenEpochKeyStore: %v", err)
	}

	containerID := tag.NewID()
	epochID := tag.NewID()
	putEpochKey(t, eks, containerID, epochID, safe.KeyRole_ContentKey, randomKeyBytes(t))
	if err := eks.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := eks.GetKey(containerID, epochID, safe.KeyRole_ContentKey); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("GetKey on closed store: got %v, want safe.ErrStoreClosed", err)
	}
	if _, err := eks.GetCurrentKey(containerID, safe.KeyRole_ContentKey); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("GetCurrentKey on closed store: got %v, want safe.ErrStoreClosed", err)
	}
	if err := eks.PutKey(ctx, containerID, safe.SymKey{EpochID: epochID, Bytes: randomKeyBytes(t)}); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("PutKey on closed store: got %v, want safe.ErrStoreClosed", err)
	}
	if err := eks.SetCurrentEpoch(ctx, containerID, epochID); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("SetCurrentEpoch on closed store: got %v, want safe.ErrStoreClosed", err)
	}
	if err := eks.ShredKeys(ctx, []tag.UID{epochID}); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("ShredKeys on closed store: got %v, want safe.ErrStoreClosed", err)
	}
}
