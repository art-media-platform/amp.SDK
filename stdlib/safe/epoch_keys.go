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
// Each epoch gets one EpochKeyEntry carrying up to 4 role-tagged materials (see KeyRole).
// Map is keyed by EpochID; role lookup is a short linear scan within the entry.
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

	// All epoch entries indexed by epochID for O(1) lookup.
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

func (eks *epochKeyStore) PutKey(containerID tag.UID, key SymKey) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()

	if eks.closed {
		return fmt.Errorf("safe: epoch key store is closed")
	}
	if !key.EpochID.IsSet() {
		return fmt.Errorf("safe: PutKey requires a non-zero EpochID")
	}

	keyCopy := append([]byte(nil), key.Bytes...)

	// Merge into the existing epoch entry if present; otherwise create one.
	entry, ok := eks.keys[key.EpochID]
	if !ok {
		entry = &EpochKeyEntry{
			ContainerID_0: containerID[0],
			ContainerID_1: containerID[1],
			EpochID_0:     key.EpochID[0],
			EpochID_1:     key.EpochID[1],
			CryptoKitID:   key.CryptoKitID,
		}
		eks.keys[key.EpochID] = entry
	}

	// Upsert the role within this epoch's RoleKeys.
	placed := false
	for i, rk := range entry.RoleKeys {
		if rk.Role == key.Role {
			Zero(entry.RoleKeys[i].Key)
			entry.RoleKeys[i].Key = keyCopy
			placed = true
			break
		}
	}
	if !placed {
		entry.RoleKeys = append(entry.RoleKeys, &RoleKey{
			Role: key.Role,
			Key:  keyCopy,
		})
	}

	// Auto-set as current if no current epoch exists or if this is newer
	if cur, ok := eks.current[containerID]; !ok {
		eks.current[containerID] = key.EpochID
	} else {
		if key.EpochID[0] > cur[0] || (key.EpochID[0] == cur[0] && key.EpochID[1] > cur[1]) {
			eks.current[containerID] = key.EpochID
		}
	}

	eks.changed = true
	return nil
}

func (eks *epochKeyStore) GetKey(containerID, epochID tag.UID, role KeyRole) (SymKey, error) {
	eks.mu.RLock()
	defer eks.mu.RUnlock()

	if eks.closed {
		return SymKey{}, fmt.Errorf("safe: epoch key store is closed")
	}

	entry, ok := eks.keys[epochID]
	if !ok {
		return SymKey{}, status.Code_KeyringNotFound.Errorf("epoch key not found: %s", epochID.Base32())
	}
	for _, rk := range entry.RoleKeys {
		if rk.Role == role {
			return SymKey{
				CryptoKitID: entry.CryptoKitID,
				EpochID:     epochID,
				Role:        rk.Role,
				Bytes:       append([]byte(nil), rk.Key...),
			}, nil
		}
	}
	return SymKey{}, status.Code_KeyringNotFound.Errorf("epoch key role not found: %s role=%s", epochID.Base32(), role)
}

func (eks *epochKeyStore) GetCurrentKey(containerID tag.UID, role KeyRole) (SymKey, error) {
	eks.mu.RLock()
	defer eks.mu.RUnlock()

	if eks.closed {
		return SymKey{}, fmt.Errorf("safe: epoch key store is closed")
	}

	epochID, ok := eks.current[containerID]
	if !ok {
		return SymKey{}, status.Code_KeyringNotFound.Errorf("no current epoch for container %s", containerID.Base32())
	}

	entry, ok := eks.keys[epochID]
	if !ok {
		return SymKey{}, status.Code_KeyringNotFound.Errorf("current epoch key missing: %s", epochID.Base32())
	}
	for _, rk := range entry.RoleKeys {
		if rk.Role == role {
			return SymKey{
				CryptoKitID: entry.CryptoKitID,
				EpochID:     epochID,
				Role:        rk.Role,
				Bytes:       append([]byte(nil), rk.Key...),
			}, nil
		}
	}
	return SymKey{}, status.Code_KeyringNotFound.Errorf("current epoch key role missing: %s role=%s", epochID.Base32(), role)
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
		for _, rk := range entry.RoleKeys {
			Zero(rk.Key)
		}
	}
	eks.keys = nil
	eks.current = nil
	Zero(eks.aad)
}
