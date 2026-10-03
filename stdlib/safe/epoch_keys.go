package safe

import (
	"context"
	"crypto/subtle"
	"fmt"
	"reflect"
	"sync"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

// TomeStoreIdentity is optional for stores whose separately constructed handles
// share storage. Wrappers should forward the identity of their underlying store.
// Without it, coordination covers handles using the same comparable TomeStore.
type TomeStoreIdentity interface {
	TomeStoreIdentity() string
}

type epochTomeLock struct {
	mu       sync.Mutex
	refs     int
	revision uint64
}

var epochTomeLocks = struct {
	sync.Mutex
	byID map[any]*epochTomeLock
}{byID: make(map[any]*epochTomeLock)}

func acquireEpochTome(store TomeStore) (any, *epochTomeLock, error) {
	identity := any(store)
	if named, ok := store.(TomeStoreIdentity); ok {
		identity = named.TomeStoreIdentity()
		if identity == "" {
			return nil, nil, fmt.Errorf("safe: empty tome identity")
		}
	} else if store == nil || !reflect.TypeOf(store).Comparable() {
		return nil, nil, fmt.Errorf("safe: tome requires a stable comparable identity")
	}
	epochTomeLocks.Lock()
	defer epochTomeLocks.Unlock()
	shared := epochTomeLocks.byID[identity]
	if shared == nil {
		shared = &epochTomeLock{}
		epochTomeLocks.byID[identity] = shared
	}
	shared.refs++
	return identity, shared, nil
}

func releaseEpochTome(identity any) {
	epochTomeLocks.Lock()
	defer epochTomeLocks.Unlock()
	shared := epochTomeLocks.byID[identity]
	shared.refs--
	if shared.refs == 0 {
		delete(epochTomeLocks.byID, identity)
	}
}

type epochAddress struct {
	scope KeyScope
	epoch tag.UID
}

type ownershipAddress struct {
	planet tag.UID
	epoch  tag.UID
}

type epochOwnership struct {
	channel  tag.UID
	planet   bool
	conflict bool
}

type epochState struct {
	keys    map[epochAddress]*EpochKeyEntry
	current map[KeyScope]tag.UID
	owners  map[ownershipAddress]epochOwnership
}

func newEpochState() *epochState {
	return &epochState{
		keys:    make(map[epochAddress]*EpochKeyEntry),
		current: make(map[KeyScope]tag.UID),
		owners:  make(map[ownershipAddress]epochOwnership),
	}
}

// epochKeyStore serializes refresh/mutation/save across live handles of a tome.
// Disk is authoritative at each mutation; stale sibling memory never replaces a
// newer role. Ownership entries survive shredding. Reads refresh after sibling
// writes, so newly ambiguous channel ownership cannot remain silently usable.
// This is process-local coordination, not cross-process locking or revocation.
type epochKeyStore struct {
	mu       sync.Mutex
	store    TomeStore
	guard    Guard
	aad      []byte
	closed   bool
	changed  bool
	identity any
	shared   *epochTomeLock
	revision uint64
	state    *epochState
	shredded map[epochAddress]struct{}
}

var _ EpochKeyStore = (*epochKeyStore)(nil)

func OpenEpochKeyStore(ctx context.Context, store TomeStore, guard Guard, aad []byte) (EpochKeyStore, error) {
	identity, shared, err := acquireEpochTome(store)
	if err != nil {
		return nil, err
	}
	eks := &epochKeyStore{
		store:    store,
		guard:    guard,
		aad:      append([]byte(nil), aad...),
		identity: identity,
		shared:   shared,
		shredded: make(map[epochAddress]struct{}),
	}
	shared.mu.Lock()
	eks.state, err = eks.loadState(ctx)
	eks.revision = shared.revision
	shared.mu.Unlock()
	if err != nil {
		Zero(eks.aad)
		releaseEpochTome(identity)
		return nil, err
	}
	return eks, nil
}

func validateKeyScope(scope KeyScope) error {
	if scope.PlanetID.IsNil() {
		return status.Code_BadRequest.Error("safe: key scope requires an owning planet")
	}
	switch scope.Kind {
	case EpochKeyScope_ScopePlanet:
		if scope.ChannelID.IsSet() {
			return status.Code_BadRequest.Error("safe: planet scope cannot name a channel")
		}
	case EpochKeyScope_ScopeChannel:
		if scope.ChannelID.IsNil() {
			return status.Code_BadRequest.Error("safe: channel scope requires a channel")
		}
	default:
		return status.Code_BadRequest.Error("safe: invalid key scope kind")
	}
	return nil
}

func storedScope(kind EpochKeyScope, planetID, containerID tag.UID) (KeyScope, error) {
	scope := Scope(planetID)
	switch kind {
	case EpochKeyScope_ScopePlanet:
		if containerID != planetID {
			return KeyScope{}, fmt.Errorf("safe: invalid planet key ownership")
		}
	case EpochKeyScope_ScopeChannel:
		if containerID.IsNil() {
			return KeyScope{}, fmt.Errorf("safe: missing channel key owner")
		}
		scope = ChannelScope(planetID, containerID)
	default:
		return KeyScope{}, fmt.Errorf("safe: unspecified epoch key ownership")
	}
	return scope, validateKeyScope(scope)
}

func (scope KeyScope) stored() (EpochKeyScope, tag.UID) {
	if scope.Kind == EpochKeyScope_ScopeChannel {
		return scope.Kind, scope.ChannelID
	}
	return scope.Kind, scope.PlanetID
}

func (eks *epochKeyStore) loadState(ctx context.Context) (*epochState, error) {
	return eks.loadStateExcept(ctx, nil)
}

func (eks *epochKeyStore) loadStateExcept(ctx context.Context, preserve *epochAddress) (*epochState, error) {
	state := newEpochState()
	sealed, err := eks.store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("safe: load epoch tome: %w", err)
	}
	if sealed == nil {
		return state, nil
	}
	dek, err := eks.guard.UnwrapDEK(ctx, sealed.WrappedDEK, eks.aad)
	if err != nil {
		return nil, err
	}
	defer Zero(dek)
	plain, err := OpenAEAD(dek, sealed.TomeNonce, sealed.Cipherblob, eks.aad)
	if err != nil {
		return nil, err
	}
	defer Zero(plain)
	tome := &EpochKeyTome{}
	if err := proto.Unmarshal(plain, tome); err != nil {
		return nil, err
	}
	valid := false
	defer func() {
		if !valid {
			for _, entry := range tome.Keys {
				zeroEntry(entry)
			}
		}
	}()
	for _, entry := range tome.Keys {
		if entry == nil || entry.EpochID().IsNil() {
			return nil, fmt.Errorf("safe: invalid epoch ownership entry")
		}
		scope, err := storedScope(entry.Scope, tag.UID{entry.PlanetID_0, entry.PlanetID_1}, entry.ContainerID())
		if err != nil {
			return nil, err
		}
		address := epochAddress{scope: scope, epoch: entry.EpochID()}
		if _, duplicate := state.keys[address]; duplicate {
			return nil, fmt.Errorf("safe: duplicate scoped epoch entry")
		}
		if len(entry.RoleKeys) > 0 && entry.CryptoKitID().IsNil() {
			return nil, fmt.Errorf("safe: epoch material has no crypto kit")
		}
		roles := make(map[KeyRole]struct{})
		for _, roleKey := range entry.RoleKeys {
			if roleKey == nil || len(roleKey.Key) == 0 || roleKey.Role < KeyRole_ContentKey || roleKey.Role > KeyRole_ReservedRole3 {
				return nil, fmt.Errorf("safe: invalid epoch role material")
			}
			if _, duplicate := roles[roleKey.Role]; duplicate {
				return nil, fmt.Errorf("safe: duplicate epoch role")
			}
			roles[roleKey.Role] = struct{}{}
		}
		state.keys[address] = entry
		if len(entry.RoleKeys) > 0 && entry.EpochID().CompareTo(state.current[scope]) > 0 {
			state.current[scope] = entry.EpochID()
		}
	}
	electedScopes := make(map[KeyScope]struct{})
	for _, election := range tome.Current {
		if election == nil {
			return nil, fmt.Errorf("safe: invalid epoch election")
		}
		scope, err := storedScope(election.Scope, tag.UID{election.PlanetID_0, election.PlanetID_1}, election.ContainerID())
		if err != nil {
			return nil, err
		}
		if _, duplicate := electedScopes[scope]; duplicate {
			return nil, fmt.Errorf("safe: duplicate scoped epoch election")
		}
		electedScopes[scope] = struct{}{}
		epochID := election.EpochID()
		if epochID.IsSet() {
			entry := state.keys[epochAddress{scope: scope, epoch: epochID}]
			if entry == nil || len(entry.RoleKeys) == 0 {
				return nil, fmt.Errorf("safe: election names no scoped key material")
			}
		}
		state.current[scope] = epochID
	}
	for address := range eks.shredded {
		if preserve != nil && address == *preserve {
			continue
		}
		state.shred(address)
	}
	state.rebuildOwners()
	valid = true
	return state, nil
}

func (state *epochState) rebuildOwners() {
	state.owners = make(map[ownershipAddress]epochOwnership)
	for address := range state.keys {
		ownerID := ownershipAddress{planet: address.scope.PlanetID, epoch: address.epoch}
		owner := state.owners[ownerID]
		if address.scope.Kind == EpochKeyScope_ScopePlanet {
			owner.planet = true
		} else if owner.channel.IsNil() {
			owner.channel = address.scope.ChannelID
		} else if owner.channel != address.scope.ChannelID {
			owner.conflict = true
		}
		state.owners[ownerID] = owner
	}
}

func zeroEntry(entry *EpochKeyEntry) {
	if entry != nil {
		for _, roleKey := range entry.RoleKeys {
			if roleKey != nil {
				Zero(roleKey.Key)
			}
		}
	}
}

func (state *epochState) zero() {
	if state != nil {
		for _, entry := range state.keys {
			zeroEntry(entry)
		}
	}
}

func (entry *EpochKeyEntry) roleKey(role KeyRole) *RoleKey {
	if entry != nil {
		for _, roleKey := range entry.RoleKeys {
			if roleKey.Role == role {
				return roleKey
			}
		}
	}
	return nil
}

func (eks *epochKeyStore) publish(state *epochState) {
	eks.state.zero()
	eks.state = state
	eks.revision = eks.shared.revision
}

func (eks *epochKeyStore) refreshLocked(ctx context.Context) error {
	if eks.revision == eks.shared.revision {
		return nil
	}
	state, err := eks.loadState(ctx)
	if err != nil {
		return err
	}
	eks.publish(state)
	return nil
}

func (eks *epochKeyStore) PutKey(ctx context.Context, scope KeyScope, key SymKey) error {
	_, err := eks.putKey(ctx, scope, key, false)
	return err
}

func (eks *epochKeyStore) InstallKey(ctx context.Context, scope KeyScope, key SymKey) (bool, error) {
	return eks.putKey(ctx, scope, key, true)
}

func (eks *epochKeyStore) putKey(ctx context.Context, scope KeyScope, key SymKey, compare bool) (bool, error) {
	eks.mu.Lock()
	defer eks.mu.Unlock()
	if eks.closed {
		return false, ErrStoreClosed
	}
	if err := validateKeyScope(scope); err != nil {
		return false, err
	}
	if key.EpochID.IsNil() || len(key.Bytes) == 0 || key.CryptoKitID.IsNil() || key.Role < KeyRole_ContentKey || key.Role > KeyRole_ReservedRole3 {
		return false, status.Code_BadRequest.Error("safe: epoch key requires epoch, kit, material and a supported role")
	}
	eks.shared.mu.Lock()
	defer eks.shared.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	address := epochAddress{scope: scope, epoch: key.EpochID}
	state, err := eks.loadStateExcept(ctx, &address)
	if err != nil {
		return false, err
	}
	defer func() {
		if state != nil {
			state.zero()
		}
	}()
	entry := state.keys[address]
	held := entry.roleKey(key.Role)
	installed := held == nil
	if compare && held != nil && (entry.CryptoKitID() != key.CryptoKitID || subtle.ConstantTimeCompare(held.Key, key.Bytes) != 1) {
		return false, status.Code_AuthFailed.Error("safe: scoped epoch key conflicts with held material")
	}
	if entry == nil {
		kind, container := scope.stored()
		entry = &EpochKeyEntry{
			Scope:         kind,
			PlanetID_0:    scope.PlanetID[0],
			PlanetID_1:    scope.PlanetID[1],
			ContainerID_0: container[0],
			ContainerID_1: container[1],
			EpochID_0:     key.EpochID[0],
			EpochID_1:     key.EpochID[1],
			CryptoKitID_0: key.CryptoKitID[0],
			CryptoKitID_1: key.CryptoKitID[1],
		}
		state.keys[address] = entry
	} else if len(entry.RoleKeys) > 0 && entry.CryptoKitID() != key.CryptoKitID {
		return false, status.Code_AuthFailed.Error("safe: scoped epoch crypto kit conflicts")
	}
	entry.SetCryptoKitID(key.CryptoKitID)
	if held != nil {
		Zero(held.Key)
		held.Key = append([]byte(nil), key.Bytes...)
	} else {
		entry.RoleKeys = append(entry.RoleKeys, &RoleKey{Role: key.Role, Key: append([]byte(nil), key.Bytes...)})
	}
	if !compare || installed {
		if current := state.current[scope]; current.IsNil() || key.EpochID.CompareTo(current) > 0 {
			state.current[scope] = key.EpochID
		}
	}
	state.rebuildOwners()
	if err := eks.saveState(ctx, state); err != nil {
		return false, err
	}
	eks.shredded = make(map[epochAddress]struct{})
	eks.changed = false
	eks.publish(state)
	state = nil
	return installed, nil
}

func (eks *epochKeyStore) GetKey(scope KeyScope, epochID tag.UID, role KeyRole) (SymKey, error) {
	eks.mu.Lock()
	defer eks.mu.Unlock()
	if eks.closed {
		return SymKey{}, ErrStoreClosed
	}
	if err := validateKeyScope(scope); err != nil {
		return SymKey{}, err
	}
	eks.shared.mu.Lock()
	defer eks.shared.mu.Unlock()
	if err := eks.refreshLocked(context.Background()); err != nil {
		return SymKey{}, err
	}
	return eks.state.get(scope, epochID, role)
}

func (state *epochState) get(scope KeyScope, epochID tag.UID, role KeyRole) (SymKey, error) {
	entry := state.keys[epochAddress{scope: scope, epoch: epochID}]
	if roleKey := entry.roleKey(role); roleKey != nil {
		return SymKey{
			CryptoKitID: entry.CryptoKitID(),
			EpochID:     epochID,
			Role:        role,
			Bytes:       append([]byte(nil), roleKey.Key...),
		}, nil
	}
	return SymKey{}, status.Code_KeyringNotFound.Error("safe: scoped epoch key not found")
}

func (eks *epochKeyStore) GetCurrentKey(scope KeyScope, role KeyRole) (SymKey, error) {
	eks.mu.Lock()
	defer eks.mu.Unlock()
	if eks.closed {
		return SymKey{}, ErrStoreClosed
	}
	if err := validateKeyScope(scope); err != nil {
		return SymKey{}, err
	}
	eks.shared.mu.Lock()
	defer eks.shared.mu.Unlock()
	if err := eks.refreshLocked(context.Background()); err != nil {
		return SymKey{}, err
	}
	return eks.state.get(scope, eks.state.current[scope], role)
}

func (eks *epochKeyStore) ResolveChannelScope(planetID, epochID tag.UID) (KeyScope, error) {
	eks.mu.Lock()
	defer eks.mu.Unlock()
	if eks.closed {
		return KeyScope{}, ErrStoreClosed
	}
	if err := validateKeyScope(Scope(planetID)); err != nil {
		return KeyScope{}, err
	}
	eks.shared.mu.Lock()
	defer eks.shared.mu.Unlock()
	if err := eks.refreshLocked(context.Background()); err != nil {
		return KeyScope{}, err
	}
	owner := eks.state.owners[ownershipAddress{planet: planetID, epoch: epochID}]
	if owner.planet || owner.conflict {
		return KeyScope{}, status.Code_AuthFailed.Error("safe: ambiguous channel epoch ownership")
	}
	if owner.channel.IsNil() {
		return KeyScope{}, status.Code_KeyringNotFound.Error("safe: channel epoch ownership not found")
	}
	return ChannelScope(planetID, owner.channel), nil
}

func (eks *epochKeyStore) SetCurrentEpoch(ctx context.Context, scope KeyScope, epochID tag.UID) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()
	if eks.closed {
		return ErrStoreClosed
	}
	if err := validateKeyScope(scope); err != nil {
		return err
	}
	eks.shared.mu.Lock()
	defer eks.shared.mu.Unlock()
	state, err := eks.loadState(ctx)
	if err != nil {
		return err
	}
	entry := state.keys[epochAddress{scope: scope, epoch: epochID}]
	if entry == nil || len(entry.RoleKeys) == 0 {
		state.zero()
		return status.Code_KeyringNotFound.Error("safe: cannot elect absent scoped epoch")
	}
	state.current[scope] = epochID
	if err := eks.saveState(ctx, state); err != nil {
		state.zero()
		return err
	}
	eks.changed = false
	eks.shredded = make(map[epochAddress]struct{})
	eks.publish(state)
	return nil
}

func (state *epochState) shred(address epochAddress) {
	if entry := state.keys[address]; entry != nil {
		zeroEntry(entry)
		entry.RoleKeys = nil
	}
	if state.current[address.scope] == address.epoch {
		state.current[address.scope] = tag.UID{}
	}
}

func (eks *epochKeyStore) ShredKeys(ctx context.Context, scope KeyScope, epochIDs []tag.UID) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()
	if eks.closed {
		return ErrStoreClosed
	}
	if err := validateKeyScope(scope); err != nil {
		return err
	}
	eks.shared.mu.Lock()
	defer eks.shared.mu.Unlock()
	state, err := eks.loadState(ctx)
	if err != nil {
		return err
	}
	for _, epochID := range epochIDs {
		address := epochAddress{scope: scope, epoch: epochID}
		if state.keys[address] != nil {
			eks.shredded[address] = struct{}{}
			state.shred(address)
		}
	}
	eks.changed = true
	err = eks.saveState(ctx, state)
	eks.publish(state)
	if err == nil {
		eks.changed = false
		eks.shredded = make(map[epochAddress]struct{})
	}
	return err
}

func (eks *epochKeyStore) Close(ctx context.Context) error {
	eks.mu.Lock()
	defer eks.mu.Unlock()
	if eks.closed {
		return nil
	}
	eks.shared.mu.Lock()
	defer eks.shared.mu.Unlock()
	if eks.changed {
		state, err := eks.loadState(ctx)
		if err != nil {
			return err
		}
		err = eks.saveState(ctx, state)
		state.zero()
		if err != nil {
			return err
		}
	}
	eks.state.zero()
	eks.state = nil
	eks.shredded = nil
	Zero(eks.aad)
	eks.closed = true
	releaseEpochTome(eks.identity)
	return nil
}

// saveState is called under both the session and shared tome locks. Even an
// error changes the revision: a backend can publish before returning an error.
func (eks *epochKeyStore) saveState(ctx context.Context, state *epochState) error {
	defer func() { eks.shared.revision++ }()
	if err := ctx.Err(); err != nil {
		return err
	}
	tome := &EpochKeyTome{
		Revision: 2,
		Keys:     make([]*EpochKeyEntry, 0, len(state.keys)),
		Current:  make([]*EpochElection, 0, len(state.current)),
	}
	for _, entry := range state.keys {
		tome.Keys = append(tome.Keys, entry)
	}
	for scope, epochID := range state.current {
		kind, container := scope.stored()
		tome.Current = append(tome.Current, &EpochElection{
			Scope:         kind,
			PlanetID_0:    scope.PlanetID[0],
			PlanetID_1:    scope.PlanetID[1],
			ContainerID_0: container[0],
			ContainerID_1: container[1],
			EpochID_0:     epochID[0],
			EpochID_1:     epochID[1],
		})
	}
	plain, err := proto.Marshal(tome)
	if err != nil {
		return err
	}
	defer Zero(plain)
	dek, err := GenerateDEK(RandReader)
	if err != nil {
		return err
	}
	defer Zero(dek)
	nonce, cipherblob, err := SealAEAD(RandReader, dek, plain, eks.aad)
	if err != nil {
		return err
	}
	wrapped, err := eks.guard.WrapDEK(ctx, dek, eks.aad)
	if err != nil {
		return err
	}
	return eks.store.Save(ctx, &SealedTome{
		Version:    uint32(Const_SealedTomeVersion),
		WrappedDEK: wrapped,
		Purpose:    "epoch-keys",
		TomeCipher: CipherName,
		TomeNonce:  nonce,
		Cipherblob: cipherblob,
	})
}
