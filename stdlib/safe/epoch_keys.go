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
// All epoch keys are loaded on Open via the same Guard/TomeStore mechanism used by the
// identity Enclave.  Mutations are durable at return: PutKey, SetCurrentEpoch, and
// ShredKeys persist the re-sealed tome before reporting success, so an unclean kill
// never loses an installed key or regresses a current-epoch election (a founded
// planet's only ContentKey copy must not ride on a clean Close).
//
// Several live stores may share one tome (two sessions of the same member).
// Every persist is a load-before-persist union: keys and elections the tome
// gained since this store loaded are merged in (and adopted into memory), and
// an epoch this store shredded is never re-adopted from the tome — so no
// session's persist drops a sibling's install, and a store's own shred holds
// across its own later persists.  (A sibling that still holds a shredded key
// in memory re-adds it at its next persist; closing that window needs a
// persisted, timestamped tombstone.)  For extreme scale (millions of
// historical keys), a future implementation can add LRU eviction and lazy
// disk loading — the interface is unchanged.
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

	// Epochs this store shredded — never re-adopted from the tome by the
	// persist union; a later PutKey of the epoch is a deliberate re-install
	// and lifts the mark.  Session-local.
	shredded map[tag.UID]struct{}
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
		store:    store,
		guard:    guard,
		aad:      append([]byte(nil), aad...),
		keys:     make(map[tag.UID]*EpochKeyEntry),
		current:  make(map[tag.UID]tag.UID),
		shredded: make(map[tag.UID]struct{}),
	}

	tome, err := eks.loadTome(ctx)
	if err != nil {
		return nil, err
	}
	if tome != nil {
		eks.mergeTome(tome)
	}
	return eks, nil
}

// loadTome reads and opens the persisted tome; nil when the store holds none.
func (eks *epochKeyStore) loadTome(ctx context.Context) (*EpochKeyTome, error) {
	sealed, err := eks.store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to load epoch key store: %w", err)
	}
	if sealed == nil {
		return nil, nil
	}

	dek, err := eks.guard.UnwrapDEK(ctx, sealed.WrappedDEK, eks.aad)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to unwrap epoch key DEK: %w", err)
	}
	defer Zero(dek)

	tomeBytes, err := OpenAEAD(dek, sealed.TomeNonce, sealed.Cipherblob, eks.aad)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to decrypt epoch key store: %w", err)
	}
	defer Zero(tomeBytes)

	tome := &EpochKeyTome{}
	if err := proto.Unmarshal(tomeBytes, tome); err != nil {
		return nil, fmt.Errorf("safe: failed to unmarshal epoch key store: %w", err)
	}
	return tome, nil
}

// mergeTome unions a persisted tome into memory — the one site for Open and
// for the load-before-persist union: keys the tome holds that memory lacks
// (per role; an epoch this store shredded is skipped), then elections for
// containers memory had not elected before this merge.  Memory wins where
// both hold a value: this store's installs and elections are its own acts,
// and a container whose election this store dropped (a shredded current) is
// never silently re-elected from the tome.  Caller holds eks.mu (or is Open,
// before the store is shared).
func (eks *epochKeyStore) mergeTome(tome *EpochKeyTome) {
	preElected := make(map[tag.UID]struct{}, len(eks.current))
	for containerID := range eks.current {
		preElected[containerID] = struct{}{}
	}

	for _, entry := range tome.Keys {
		epochID := entry.EpochID()
		if _, gone := eks.shredded[epochID]; gone {
			continue
		}
		containerID := entry.ContainerID()
		if held, ok := eks.keys[epochID]; ok {
			for _, rk := range entry.RoleKeys {
				if held.roleKey(rk.Role) == nil {
					held.RoleKeys = append(held.RoleKeys, rk)
				}
			}
			continue
		}
		eks.keys[epochID] = entry

		// An adopted key follows PutKey's rule: newest epoch per container is
		// the fallback election (EpochID is time-based, so larger = newer).
		if cur, ok := eks.current[containerID]; !ok {
			eks.current[containerID] = epochID
		} else if epochID[0] > cur[0] || (epochID[0] == cur[0] && epochID[1] > cur[1]) {
			eks.current[containerID] = epochID
		}
	}

	// A persisted election overrides the newest-per-container fallback —
	// SetCurrentEpoch is durable at return, so an explicit election (possibly
	// of an older epoch) survives reopen.  Only for containers this store had
	// not elected before the merge; an election at an epoch no longer held is
	// skipped (fail-closed: the fallback stands).
	for _, elected := range tome.Current {
		containerID := elected.ContainerID()
		if _, own := preElected[containerID]; own {
			continue
		}
		epochID := elected.EpochID()
		if _, held := eks.keys[epochID]; held {
			eks.current[containerID] = epochID
		}
	}

	// A current pointer at a shredded epoch dangles — drop it (fail-closed:
	// no silent re-election; PutKey or SetCurrentEpoch names a successor).
	for containerID, epochID := range eks.current {
		if _, held := eks.keys[epochID]; !held {
			delete(eks.current, containerID)
		}
	}
}

// roleKey returns the entry's material for role, or nil.
func (entry *EpochKeyEntry) roleKey(role KeyRole) *RoleKey {
	for _, rk := range entry.RoleKeys {
		if rk.Role == role {
			return rk
		}
	}
	return nil
}

func (eks *epochKeyStore) PutKey(ctx context.Context, containerID tag.UID, key SymKey) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()

	if eks.closed {
		return ErrStoreClosed
	}
	if !key.EpochID.IsSet() {
		return fmt.Errorf("safe: PutKey requires a non-zero EpochID")
	}

	keyCopy := append([]byte(nil), key.Bytes...)
	delete(eks.shredded, key.EpochID) // a deliberate re-install lifts the shred mark

	// Merge into the existing epoch entry if present; otherwise create one.
	entry, ok := eks.keys[key.EpochID]
	if !ok {
		entry = &EpochKeyEntry{
			ContainerID_0: containerID[0],
			ContainerID_1: containerID[1],
			EpochID_0:     key.EpochID[0],
			EpochID_1:     key.EpochID[1],
			CryptoKitID_0: key.CryptoKitID[0],
			CryptoKitID_1: key.CryptoKitID[1],
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

	// Dirty BEFORE the persist attempt (the ShredKeys discipline): a failed
	// Save returns the error with the install tracked as unsaved, so both a
	// caller retry and a later Close carry it to disk.
	eks.changed = true
	return eks.persistLocked(ctx)
}

func (eks *epochKeyStore) GetKey(containerID, epochID tag.UID, role KeyRole) (SymKey, error) {
	eks.mu.RLock()
	defer eks.mu.RUnlock()

	if eks.closed {
		return SymKey{}, ErrStoreClosed
	}

	entry, ok := eks.keys[epochID]
	if !ok {
		return SymKey{}, status.Code_KeyringNotFound.Errorf("epoch key not found: %s", epochID.Base32())
	}
	for _, rk := range entry.RoleKeys {
		if rk.Role == role {
			return SymKey{
				CryptoKitID: entry.CryptoKitID(),
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
		return SymKey{}, ErrStoreClosed
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
				CryptoKitID: entry.CryptoKitID(),
				EpochID:     epochID,
				Role:        rk.Role,
				Bytes:       append([]byte(nil), rk.Key...),
			}, nil
		}
	}
	return SymKey{}, status.Code_KeyringNotFound.Errorf("current epoch key role missing: %s role=%s", epochID.Base32(), role)
}

// SetCurrentEpoch implements EpochKeyStore: the election persists before
// return — it may name an OLDER epoch, which the newest-per-container reopen
// fallback would otherwise regress after a crash.
func (eks *epochKeyStore) SetCurrentEpoch(ctx context.Context, containerID, epochID tag.UID) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()

	if eks.closed {
		return ErrStoreClosed
	}

	if _, ok := eks.keys[epochID]; !ok {
		return status.Code_KeyringNotFound.Errorf("cannot set current: epoch key %s not found", epochID.Base32())
	}

	eks.current[containerID] = epochID

	// Dirty BEFORE the persist attempt (the PutKey/ShredKeys discipline): a
	// failed Save returns the error with the election tracked as unsaved, so
	// both a caller retry and a later Close carry it to disk.
	eks.changed = true
	return eks.persistLocked(ctx)
}

// ShredKeys implements EpochKeyStore: the removal persists before return —
// the durability half of an operator HistoryCut (cut keys are losable by
// ruling, so destruction, not recovery, is the obligation here).
func (eks *epochKeyStore) ShredKeys(ctx context.Context, epochIDs []tag.UID) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()

	if eks.closed {
		return ErrStoreClosed
	}

	shredded := false
	for _, epochID := range epochIDs {
		entry, ok := eks.keys[epochID]
		if !ok {
			continue
		}
		for _, rk := range entry.RoleKeys {
			Zero(rk.Key)
		}
		eks.shredded[epochID] = struct{}{}
		delete(eks.keys, epochID)
		shredded = true
	}
	if shredded {
		// Dirty BEFORE the persist attempt: a failed Save must leave the
		// removal tracked as unsaved, so both a retry and a later Close
		// reach disk — otherwise the shred could un-shred on reopen.
		eks.changed = true
	}
	if !eks.changed {
		return nil // nothing shredded AND memory == disk: the persisted tome already lacks them
	}

	// A current pointer at a shredded epoch dangles — drop it (fail-closed:
	// no silent re-election; PutKey or SetCurrentEpoch names a successor).
	for containerID, epochID := range eks.current {
		if _, held := eks.keys[epochID]; !held {
			delete(eks.current, containerID)
		}
	}

	return eks.persistLocked(ctx)
}

func (eks *epochKeyStore) Close(ctx context.Context) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()

	if eks.closed {
		return nil
	}

	if eks.changed {
		if err := eks.persistLocked(ctx); err != nil {
			return err
		}
	}

	eks.zeroKeys()
	eks.closed = true
	return nil
}

// persistLocked unions the persisted tome into memory (a sibling store may
// have written since this store loaded), then seals the in-memory map into an
// EpochKeyTome and saves it via the Guard/TomeStore pair — the one persist
// site (PutKey, SetCurrentEpoch, ShredKeys, Close).  Caller holds eks.mu.
func (eks *epochKeyStore) persistLocked(ctx context.Context) error {
	onDisk, err := eks.loadTome(ctx)
	if err != nil {
		return err
	}
	if onDisk != nil {
		eks.mergeTome(onDisk)
	}

	tome := &EpochKeyTome{
		Revision: 1,
		Keys:     make([]*EpochKeyEntry, 0, len(eks.keys)),
		Current:  make([]*EpochElection, 0, len(eks.current)),
	}
	for _, entry := range eks.keys {
		tome.Keys = append(tome.Keys, entry)
	}
	for containerID, epochID := range eks.current {
		tome.Current = append(tome.Current, &EpochElection{
			ContainerID_0: containerID[0],
			ContainerID_1: containerID[1],
			EpochID_0:     epochID[0],
			EpochID_1:     epochID[1],
		})
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

	eks.changed = false
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
	eks.shredded = nil
	Zero(eks.aad)
}
