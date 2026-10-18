package safe_test

// Persist (EnclavePersister) seals a handle's whole index again over the tome
// as it stands — the route a store that acknowledged a save without writing
// it takes once it writes again (a login gate's held save at its Release,
// SD-security-sync §8.2).

import (
	"context"
	"sync"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// holdingTomeStore acknowledges saves without writing them while holding, as
// a login gate does before its verdict, and forwards its inner store's
// identity.
type holdingTomeStore struct {
	inner   safe.TomeStore
	mu      sync.Mutex
	holding bool
}

func (store *holdingTomeStore) TomeStoreIdentity() string {
	return store.inner.(safe.TomeStoreIdentity).TomeStoreIdentity()
}

func (store *holdingTomeStore) Load(ctx context.Context) (*safe.SealedTome, error) {
	return store.inner.Load(ctx)
}

func (store *holdingTomeStore) Save(ctx context.Context, sealed *safe.SealedTome) error {
	store.mu.Lock()
	holding := store.holding
	store.mu.Unlock()
	if holding {
		return nil
	}
	return store.inner.Save(ctx, sealed)
}

func (store *holdingTomeStore) stopHolding() {
	store.mu.Lock()
	store.holding = false
	store.mu.Unlock()
}

// A handle whose install the store held persists it once the store writes
// again: the install reaches the tome beside the install a sibling saved
// meanwhile, and the sibling serves it.
func TestEnclave_PersistWritesAHeldInstallBesideASiblingSave(t *testing.T) {
	ctx := context.Background()
	inner, guard := siblingRig(t)
	holder := &holdingTomeStore{
		inner:   inner,
		holding: true,
	}
	held := openCloseTestEnclave(t, holder, guard)
	sibling := openCloseTestEnclave(t, inner, guard)
	heldRing := tag.NewID()
	generateSigningKey(t, held, heldRing)
	siblingRing := tag.NewID()
	generateSigningKey(t, sibling, siblingRing)
	if signingKeyOnDisk(t, inner, guard, heldRing) {
		t.Fatal("premise void: the holding store wrote the install")
	}

	holder.stopHolding()
	persister, ok := held.(safe.EnclavePersister)
	if !ok {
		t.Fatal("the enclave offers no Persist")
	}
	if err := persister.Persist(ctx); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if !signingKeyOnDisk(t, inner, guard, heldRing) {
		t.Error("Persist did not write the held install")
	}
	if !signingKeyOnDisk(t, inner, guard, siblingRing) {
		t.Error("Persist dropped the sibling's install from the tome")
	}
	ref := &safe.KeyRef{Type: safe.KeyType_SigningKey}
	ref.SetKeyringID(heldRing)
	if !sibling.CanSign(ref) {
		t.Error("the sibling does not serve the persisted install")
	}
	closeHandle(t, held)
	closeHandle(t, sibling)
	if err := persister.Persist(ctx); status.GetCode(err) != status.Code_Closed {
		t.Fatalf("Persist on a closed handle: %v, want Code_Closed", err)
	}
}
