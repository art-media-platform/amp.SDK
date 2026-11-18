package safe

import (
	"context"
	"crypto/subtle"
	"fmt"
	"sync"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

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
	identity any
	shared   *tomeLock
	revision uint64
	state    *epochState
}

var _ EpochKeyStore = (*epochKeyStore)(nil)

func OpenEpochKeyStore(ctx context.Context, store TomeStore, guard Guard, aad []byte) (EpochKeyStore, error) {
	identity, shared, err := acquireTome(store)
	if err != nil {
		return nil, err
	}
	eks := &epochKeyStore{
		store:    store,
		guard:    guard,
		aad:      append([]byte(nil), aad...),
		identity: identity,
		shared:   shared,
	}
	shared.mu.Lock()
	eks.state, err = eks.loadState(ctx)
	eks.revision = shared.revision.Load()
	shared.mu.Unlock()
	if err != nil {
		Zero(eks.aad)
		releaseTome(identity)
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
		address := epochAddress{
			scope: scope,
			epoch: entry.EpochID(),
		}
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
			address := epochAddress{
				scope: scope,
				epoch: epochID,
			}
			entry := state.keys[address]
			if entry == nil || len(entry.RoleKeys) == 0 {
				return nil, fmt.Errorf("safe: election names no scoped key material")
			}
		}
		state.current[scope] = epochID
	}
	for address := range eks.shared.shredded {
		state.shred(address)
	}
	state.rebuildOwners()
	valid = true
	return state, nil
}

func (state *epochState) rebuildOwners() {
	state.owners = make(map[ownershipAddress]epochOwnership)
	for address := range state.keys {
		ownerID := ownershipAddress{
			planet: address.scope.PlanetID,
			epoch:  address.epoch,
		}
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
	eks.revision = eks.shared.revision.Load()
}

func (eks *epochKeyStore) refreshLocked(ctx context.Context) error {
	if eks.revision == eks.shared.revision.Load() {
		return nil
	}
	state, err := eks.loadState(ctx)
	if err != nil {
		return err
	}
	eks.publish(state)
	return nil
}

// putMode is how putKey treats held material and the scope's election.
type putMode int32

const (
	putReplace putMode = iota + 1 // PutKey: replace; elect the newest
	putInstall                    // InstallKey: install absent; elect the newest
	putStage                      // StageKey: replace; never elect
)

func (eks *epochKeyStore) PutKey(ctx context.Context, scope KeyScope, key SymKey) error {
	_, err := eks.putKey(ctx, scope, key, putReplace)
	return err
}

func (eks *epochKeyStore) InstallKey(ctx context.Context, scope KeyScope, key SymKey) (bool, error) {
	return eks.putKey(ctx, scope, key, putInstall)
}

func (eks *epochKeyStore) StageKey(ctx context.Context, scope KeyScope, key SymKey) error {
	_, err := eks.putKey(ctx, scope, key, putStage)
	return err
}

func (eks *epochKeyStore) putKey(ctx context.Context, scope KeyScope, key SymKey, mode putMode) (bool, error) {
	compare := mode == putInstall
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
	address := epochAddress{
		scope: scope,
		epoch: key.EpochID,
	}
	if _, pending := eks.shared.shredded[address]; pending {
		return false, status.Code_NotReady.Error("safe: retry pending epoch destruction before installing a key")
	}
	state, err := eks.loadState(ctx)
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
	// A PutKey or StageKey that leaves the tome as loaded writes nothing: no
	// save, no revision bump, no sibling reload; the handle serves the loaded
	// tome.  An InstallKey always saves; a pending destruction elsewhere still
	// saves.  A held role's kit already matched (refused below otherwise).
	unchanged := !compare && held != nil && len(eks.shared.shredded) == 0 && subtle.ConstantTimeCompare(held.Key, key.Bytes) == 1
	priorElection, priorElected := state.current[scope]
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
		entry.RoleKeys = append(entry.RoleKeys, &RoleKey{
			Role: key.Role,
			Key:  append([]byte(nil), key.Bytes...),
		})
	}
	switch {
	case mode == putStage:
		// The election as it stands, recorded explicitly: a reopen otherwise
		// elects the newest held key (loadState).
		elected := state.current[scope]
		state.current[scope] = elected
	case !compare || installed:
		if current := state.current[scope]; current.IsNil() || key.EpochID.CompareTo(current) > 0 {
			state.current[scope] = key.EpochID
		}
	}
	if election, elected := state.current[scope]; unchanged && elected == priorElected && election == priorElection {
		eks.publish(state)
		state = nil
		return installed, nil
	}
	state.rebuildOwners()
	if err := eks.saveState(ctx, state); err != nil {
		return false, err
	}
	clear(eks.shared.shredded)
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

func (eks *epochKeyStore) HeldEpochs(scope KeyScope) ([]tag.UID, error) {
	eks.mu.Lock()
	defer eks.mu.Unlock()
	if eks.closed {
		return nil, ErrStoreClosed
	}
	if err := validateKeyScope(scope); err != nil {
		return nil, err
	}
	eks.shared.mu.Lock()
	defer eks.shared.mu.Unlock()
	if err := eks.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	held := []tag.UID{}
	for address, entry := range eks.state.keys {
		if address.scope == scope && len(entry.RoleKeys) > 0 {
			held = append(held, address.epoch)
		}
	}
	return held, nil
}

func (eks *epochKeyStore) HeldScopes() ([]KeyScope, error) {
	eks.mu.Lock()
	defer eks.mu.Unlock()
	if eks.closed {
		return nil, ErrStoreClosed
	}
	eks.shared.mu.Lock()
	defer eks.shared.mu.Unlock()
	if err := eks.refreshLocked(context.Background()); err != nil {
		return nil, err
	}
	seen := map[KeyScope]struct{}{}
	held := []KeyScope{}
	for address, entry := range eks.state.keys {
		if len(entry.RoleKeys) == 0 {
			continue
		}
		if _, listed := seen[address.scope]; listed {
			continue
		}
		seen[address.scope] = struct{}{}
		held = append(held, address.scope)
	}
	return held, nil
}

func (state *epochState) get(scope KeyScope, epochID tag.UID, role KeyRole) (SymKey, error) {
	address := epochAddress{
		scope: scope,
		epoch: epochID,
	}
	entry := state.keys[address]
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
	ownerID := ownershipAddress{
		planet: planetID,
		epoch:  epochID,
	}
	owner := eks.state.owners[ownerID]
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
	address := epochAddress{
		scope: scope,
		epoch: epochID,
	}
	entry := state.keys[address]
	if entry == nil || len(entry.RoleKeys) == 0 {
		state.zero()
		return status.Code_KeyringNotFound.Error("safe: cannot elect absent scoped epoch")
	}
	state.current[scope] = epochID
	if err := eks.saveState(ctx, state); err != nil {
		state.zero()
		return err
	}
	clear(eks.shared.shredded)
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
		address := epochAddress{
			scope: scope,
			epoch: epochID,
		}
		if state.keys[address] != nil {
			eks.shared.shredded[address] = struct{}{}
			state.shred(address)
		}
	}
	err = eks.saveState(ctx, state)
	eks.publish(state)
	if err == nil {
		clear(eks.shared.shredded)
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
	err := eks.flushPendingLocked(ctx)
	eks.state.zero()
	eks.state = nil
	Zero(eks.aad)
	eks.closed = true
	releaseTome(eks.identity)
	return err
}

// flushPendingLocked persists destruction left pending by a failed save. The
// handle closes either way; the error reports that the tome still holds the
// material.
func (eks *epochKeyStore) flushPendingLocked(ctx context.Context) error {
	if len(eks.shared.shredded) == 0 {
		return nil
	}
	state, err := eks.loadState(ctx)
	if err != nil {
		return err
	}
	err = eks.saveState(ctx, state)
	state.zero()
	if err == nil {
		clear(eks.shared.shredded)
	}
	return err
}

// saveState is called under both the session and shared tome locks. Even an
// error changes the revision: a backend can publish before returning an error.
func (eks *epochKeyStore) saveState(ctx context.Context, state *epochState) error {
	defer eks.shared.revision.Add(1)
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
