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
	err := eks.PutKey(containerID, safe.SymKey{
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
	if err := eks.SetCurrentEpoch(containerID, successor); err != nil {
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
	if err := eks.PutKey(containerID, safe.SymKey{EpochID: epochID, Bytes: randomKeyBytes(t)}); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("PutKey on closed store: got %v, want safe.ErrStoreClosed", err)
	}
	if err := eks.SetCurrentEpoch(containerID, epochID); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("SetCurrentEpoch on closed store: got %v, want safe.ErrStoreClosed", err)
	}
	if err := eks.ShredKeys(ctx, []tag.UID{epochID}); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("ShredKeys on closed store: got %v, want safe.ErrStoreClosed", err)
	}
}
