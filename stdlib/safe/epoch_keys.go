package safe

import (
	"context"
	"fmt"
	"sync"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

// epochKeyStore implements EpochKeyStore with in-memory maps and encrypted-at-rest persistence.
//
// All epoch keys are loaded on Open and persisted on Close via the same Guard/TomeStore mechanism
// used by the identity Enclave.  For extreme scale (millions of historical keys), a future
// implementation can add LRU eviction and lazy disk loading — the interface is unchanged.
type epochKeyStore struct {
	mu      sync.RWMutex
	store   TomeStore
	guard   Guard
	aad     []byte
	closed  bool
	changed bool

	// All epoch keys indexed by epochID for O(1) lookup.
	keys map[tag.UID]*EpochKeyEntry

	// Tracks which epochID is current for each containerID.
	current map[tag.UID]tag.UID
}

var _ EpochKeyStore = (*epochKeyStore)(nil)

// OpenEpochKeyStore starts a new epoch key session.
// If the TomeStore has no existing data, an empty store is created.
func OpenEpochKeyStore(
	ctx context.Context,
	store TomeStore,
	guard Guard,
	aad []byte,
) (EpochKeyStore, error) {

	eks := &epochKeyStore{
		store:   store,
		guard:   guard,
		aad:     append([]byte(nil), aad...),
		keys:    make(map[tag.UID]*EpochKeyEntry),
		current: make(map[tag.UID]tag.UID),
	}

	sealed, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to load epoch key store: %w", err)
	}

	if sealed == nil {
		return eks, nil
	}

	dek, err := guard.UnwrapDEK(ctx, sealed.WrappedDEK, eks.aad)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to unwrap epoch key DEK: %w", err)
	}
	defer Zero(dek)

	tomeBytes, err := OpenAEAD(dek, sealed.TomeNonce, sealed.Cipherblob, eks.aad)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to decrypt epoch key store: %w", err)
	}
	defer Zero(tomeBytes)

	var tome EpochKeyTome
	if err := proto.Unmarshal(tomeBytes, &tome); err != nil {
		return nil, fmt.Errorf("safe: failed to unmarshal epoch key store: %w", err)
	}

	// Index all keys and identify current epochs (newest per container).
	for _, entry := range tome.Keys {
		epochID := entry.EpochID()
		containerID := entry.ContainerID()
		eks.keys[epochID] = entry

		// Track newest epoch per container (EpochID is time-based, so larger = newer)
		if cur, ok := eks.current[containerID]; !ok {
			eks.current[containerID] = epochID
		} else {
			if epochID[0] > cur[0] || (epochID[0] == cur[0] && epochID[1] > cur[1]) {
				eks.current[containerID] = epochID
			}
		}
	}

	return eks, nil
}

func (eks *epochKeyStore) PutKey(containerID, epochID tag.UID, cryptoKit CryptoKitID, key []byte) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()

	if eks.closed {
		return fmt.Errorf("safe: epoch key store is closed")
	}

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	eks.keys[epochID] = &EpochKeyEntry{
		ContainerID_0: containerID[0],
		ContainerID_1: containerID[1],
		EpochID_0:     epochID[0],
		EpochID_1:     epochID[1],
		CryptoKitID:   cryptoKit,
		Key:           keyCopy,
	}

	// Auto-set as current if no current epoch exists or if this is newer
	if cur, ok := eks.current[containerID]; !ok {
		eks.current[containerID] = epochID
	} else {
		if epochID[0] > cur[0] || (epochID[0] == cur[0] && epochID[1] > cur[1]) {
			eks.current[containerID] = epochID
		}
	}

	eks.changed = true
	return nil
}

func (eks *epochKeyStore) GetKey(containerID, epochID tag.UID) ([]byte, CryptoKitID, error) {
	eks.mu.RLock()
	defer eks.mu.RUnlock()

	if eks.closed {
		return nil, 0, fmt.Errorf("safe: epoch key store is closed")
	}

	entry, ok := eks.keys[epochID]
	if !ok {
		return nil, 0, status.Code_KeyringNotFound.Errorf("epoch key not found: %s", epochID.Base32())
	}

	out := make([]byte, len(entry.Key))
	copy(out, entry.Key)
	return out, entry.CryptoKitID, nil
}

func (eks *epochKeyStore) GetCurrentKey(containerID tag.UID) (tag.UID, []byte, error) {
	eks.mu.RLock()
	defer eks.mu.RUnlock()

	if eks.closed {
		return tag.UID{}, nil, fmt.Errorf("safe: epoch key store is closed")
	}

	epochID, ok := eks.current[containerID]
	if !ok {
		return tag.UID{}, nil, status.Code_KeyringNotFound.Errorf("no current epoch for container %s", containerID.Base32())
	}

	entry, ok := eks.keys[epochID]
	if !ok {
		return tag.UID{}, nil, status.Code_KeyringNotFound.Errorf("current epoch key missing: %s", epochID.Base32())
	}

	out := make([]byte, len(entry.Key))
	copy(out, entry.Key)
	return epochID, out, nil
}

func (eks *epochKeyStore) SetCurrentEpoch(containerID, epochID tag.UID) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()

	if eks.closed {
		return fmt.Errorf("safe: epoch key store is closed")
	}

	if _, ok := eks.keys[epochID]; !ok {
		return status.Code_KeyringNotFound.Errorf("cannot set current: epoch key %s not found", epochID.Base32())
	}

	eks.current[containerID] = epochID
	eks.changed = true
	return nil
}

func (eks *epochKeyStore) Close(ctx context.Context) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()

	if eks.closed {
		return nil
	}

	if !eks.changed {
		eks.zeroKeys()
		eks.closed = true
		return nil
	}

	// Build the EpochKeyTome from the in-memory map
	tome := &EpochKeyTome{
		Revision: 1,
		Keys:     make([]*EpochKeyEntry, 0, len(eks.keys)),
	}
	for _, entry := range eks.keys {
		tome.Keys = append(tome.Keys, entry)
	}

	tomeBytes, err := proto.Marshal(tome)
	if err != nil {
		return fmt.Errorf("safe: failed to marshal epoch key store: %w", err)
	}
	defer Zero(tomeBytes)

	dek, err := GenerateDEK(RandReader)
	if err != nil {
		return err
	}
	defer Zero(dek)

	tomeNonce, cipherblob, err := SealAEAD(RandReader, dek, tomeBytes, eks.aad)
	if err != nil {
		return fmt.Errorf("safe: failed to encrypt epoch key store: %w", err)
	}

	wrappedDEK, err := eks.guard.WrapDEK(ctx, dek, eks.aad)
	if err != nil {
		return fmt.Errorf("safe: failed to wrap epoch key DEK: %w", err)
	}

	sealed := &SealedTome{
		Version:    uint32(Const_SealedTomeVersion),
		WrappedDEK: wrappedDEK,
		Purpose:    "epoch-keys",
		TomeCipher: CipherName,
		TomeNonce:  tomeNonce,
		Cipherblob: cipherblob,
	}

	if err := eks.store.Save(ctx, sealed); err != nil {
		return fmt.Errorf("safe: failed to save epoch key store: %w", err)
	}

	eks.zeroKeys()
	eks.closed = true
	return nil
}

func (eks *epochKeyStore) zeroKeys() {
	for _, entry := range eks.keys {
		Zero(entry.Key)
	}
	eks.keys = nil
	eks.current = nil
	Zero(eks.aad)
}
