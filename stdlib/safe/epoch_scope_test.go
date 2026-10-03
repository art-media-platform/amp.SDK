package safe_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

type epochScopeFixture struct {
	path  string
	guard safe.Guard
}

// Independently encoded protobuf bytes pin field numbers, fixed64 UID halves,
// ownership discriminators, retained empty entries, and scoped elections.
// Encryption is deliberately outside the golden: its nonce is randomized.
func TestEpochKeys_ScopedTomeGolden(t *testing.T) {
	const golden = "080212570901000000000000001102000000000000001903000000000000002104000000000000002907000000000000004214121266697865642d6b65792d6d6174657269616c490100000000000000510200000000000000580112380901000000000000001102000000000000001903000000000000002104000000000000004901000000000000005102000000000000005802123809090000000000000011020000000000000019050000000000000021040000000000000049010000000000000051020000000000000058021a380901000000000000001102000000000000001903000000000000002104000000000000002901000000000000003102000000000000003801"
	plain, err := hex.DecodeString(golden)
	if err != nil {
		t.Fatal(err)
	}
	tome := &safe.EpochKeyTome{}
	if err := proto.Unmarshal(plain, tome); err != nil {
		t.Fatal(err)
	}
	if tome.Revision != 2 || len(tome.Keys) != 3 || len(tome.Current) != 1 ||
		tome.Keys[0].Scope != safe.EpochKeyScope_ScopePlanet ||
		tome.Keys[1].Scope != safe.EpochKeyScope_ScopeChannel ||
		tome.Keys[2].PlanetID_0 != 1 || tome.Current[0].Scope != safe.EpochKeyScope_ScopePlanet {
		t.Fatal("scoped persistence fixture changed interpretation")
	}
	encoded, err := proto.Marshal(tome)
	if err != nil || !bytes.Equal(encoded, plain) {
		t.Fatalf("scoped persistence fixture changed bytes: %v", err)
	}
	ctx := context.Background()
	fixture := newEpochScopeFixture(t)
	dek := bytes.Repeat([]byte{0x31}, 32)
	defer safe.Zero(dek)
	aad := []byte("scope-test")
	nonce, cipherblob, err := safe.SealAEAD(safe.RandReader, dek, plain, aad)
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := fixture.guard.WrapDEK(ctx, dek, aad)
	if err != nil {
		t.Fatal(err)
	}
	err = safe.NewLocalTomeStore(fixture.path).Save(ctx, &safe.SealedTome{
		Version:    uint32(safe.Const_SealedTomeVersion),
		WrappedDEK: wrapped,
		Purpose:    "epoch-keys",
		TomeCipher: safe.CipherName,
		TomeNonce:  nonce,
		Cipherblob: cipherblob,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := fixture.open(t)
	planet, epoch := tag.UID{1, 2}, tag.UID{3, 4}
	key, err := store.GetCurrentKey(safe.Scope(planet), safe.KeyRole_ContentKey)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Zero()
	if key.EpochID != epoch || key.CryptoKitID != (tag.UID{7, 0}) || string(key.Bytes) != "fixed-key-material" {
		t.Fatal("golden election or key material changed")
	}
	if _, err := store.ResolveChannelScope(planet, epoch); !status.IsError(err, status.Code_AuthFailed) {
		t.Fatalf("golden empty channel claim bypassed planet alias refusal: %v", err)
	}
	resolved, err := store.ResolveChannelScope(planet, tag.UID{5, 4})
	if err != nil || resolved != safe.ChannelScope(planet, tag.UID{9, 2}) {
		t.Fatalf("golden destroyed-key ownership disappeared: %v", err)
	}
}

func newEpochScopeFixture(t *testing.T) *epochScopeFixture {
	t.Helper()
	guard := safe.NewFileGuard([]byte("scope-test-password"), []byte("scope-test-guard"))
	t.Cleanup(func() { guard.Close() })
	return &epochScopeFixture{
		path:  filepath.Join(t.TempDir(), "keys.tome"),
		guard: guard,
	}
}

func (fixture *epochScopeFixture) open(t *testing.T) safe.EpochKeyStore {
	t.Helper()
	store, err := safe.OpenEpochKeyStore(context.Background(), safe.NewLocalTomeStore(fixture.path), fixture.guard, []byte("scope-test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return store
}

func scopeKey(epoch tag.UID, role safe.KeyRole, octet byte) safe.SymKey {
	return safe.SymKey{
		EpochID:     epoch,
		Role:        role,
		CryptoKitID: safe.Crypto.Poly25519.ID,
		Bytes:       bytes.Repeat([]byte{octet}, 32),
	}
}

func assertScopeKey(t *testing.T, store safe.EpochKeyStore, scope safe.KeyScope, key safe.SymKey) {
	t.Helper()
	got, err := store.GetKey(scope, key.EpochID, key.Role)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Zero()
	if got.CryptoKitID != key.CryptoKitID || !bytes.Equal(got.Bytes, key.Bytes) {
		t.Fatal("scoped key material changed")
	}
}

func TestEpochKeys_ScopeIsolationAndShred(t *testing.T) {
	ctx := context.Background()
	fixture := newEpochScopeFixture(t)
	store := fixture.open(t)
	planetA, planetB := tag.UID{11, 1}, tag.UID{12, 1}
	epoch := tag.UID{21, 1}
	scopes := []safe.KeyScope{
		safe.Scope(planetA),
		safe.Scope(planetB),
		safe.ChannelScope(planetA, planetA),
		safe.ChannelScope(planetB, planetA),
	}
	for index, scope := range scopes {
		if err := store.PutKey(ctx, scope, scopeKey(epoch, safe.KeyRole_ContentKey, byte(index+1))); err != nil {
			t.Fatal(err)
		}
	}
	// Duplicate raw container and epoch IDs remain independent after restart.
	reopened := fixture.open(t)
	for index, scope := range scopes {
		assertScopeKey(t, reopened, scope, scopeKey(epoch, safe.KeyRole_ContentKey, byte(index+1)))
	}
	if err := store.ShredKeys(ctx, scopes[0], []tag.UID{epoch}); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.GetKey(scopes[0], epoch, safe.KeyRole_ContentKey); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("sibling did not refresh shred: %v", err)
	}
	for index, scope := range scopes[1:] {
		assertScopeKey(t, reopened, scope, scopeKey(epoch, safe.KeyRole_ContentKey, byte(index+2)))
		current, err := reopened.GetCurrentKey(scope, safe.KeyRole_ContentKey)
		if err != nil || current.EpochID != epoch {
			t.Fatalf("other scope election changed: %v", err)
		}
		current.Zero()
	}
	fresh := fixture.open(t)
	if _, err := fresh.GetCurrentKey(scopes[0], safe.KeyRole_ContentKey); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("shred elected a successor: %v", err)
	}
}

func TestEpochKeys_MalformedChannelCannotAccessPlanet(t *testing.T) {
	ctx := context.Background()
	fixture := newEpochScopeFixture(t)
	store := fixture.open(t)
	planet := tag.UID{501, 1}
	epoch := tag.UID{502, 1}
	nextEpoch := tag.UID{503, 1}
	scope := safe.Scope(planet)
	key := scopeKey(epoch, safe.KeyRole_ContentKey, 0x51)
	if err := store.PutKey(ctx, scope, key); err != nil {
		t.Fatal(err)
	}
	invalid := []safe.KeyScope{
		safe.ChannelScope(planet, tag.UID{}),
		{PlanetID: planet}, // unspecified kind
		{Kind: safe.EpochKeyScope(99), PlanetID: planet},
		{Kind: safe.EpochKeyScope_ScopePlanet, PlanetID: planet, ChannelID: planet},
		safe.ChannelScope(tag.UID{}, planet),
	}
	for _, malformed := range invalid {
		if _, err := store.GetKey(malformed, epoch, safe.KeyRole_ContentKey); !status.IsError(err, status.Code_BadRequest) {
			t.Fatalf("malformed lookup accepted: %v", err)
		}
		if _, err := store.GetCurrentKey(malformed, safe.KeyRole_ContentKey); !status.IsError(err, status.Code_BadRequest) {
			t.Fatalf("malformed current lookup accepted: %v", err)
		}
		if installed, err := store.InstallKey(ctx, malformed, scopeKey(nextEpoch, safe.KeyRole_ContentKey, 0x52)); installed || !status.IsError(err, status.Code_BadRequest) {
			t.Fatalf("malformed install accepted: installed=%v err=%v", installed, err)
		}
		if err := store.PutKey(ctx, malformed, scopeKey(epoch, safe.KeyRole_ContentKey, 0x53)); !status.IsError(err, status.Code_BadRequest) {
			t.Fatalf("malformed replacement accepted: %v", err)
		}
		if err := store.SetCurrentEpoch(ctx, malformed, tag.UID{}); !status.IsError(err, status.Code_BadRequest) {
			t.Fatalf("malformed election accepted: %v", err)
		}
		if err := store.ShredKeys(ctx, malformed, []tag.UID{epoch}); !status.IsError(err, status.Code_BadRequest) {
			t.Fatalf("malformed destruction accepted: %v", err)
		}
	}
	for _, checked := range []safe.EpochKeyStore{store, fixture.open(t)} {
		assertScopeKey(t, checked, scope, key)
		current, err := checked.GetCurrentKey(scope, safe.KeyRole_ContentKey)
		if err != nil {
			t.Fatal(err)
		}
		if current.EpochID != epoch || !bytes.Equal(current.Bytes, key.Bytes) {
			t.Fatal("malformed operation changed planet election or material")
		}
		current.Zero()
		if _, err := checked.GetKey(scope, nextEpoch, safe.KeyRole_ContentKey); !status.IsError(err, status.Code_KeyringNotFound) {
			t.Fatalf("malformed install persisted a planet key: %v", err)
		}
	}
}

func TestEpochKeys_ChannelOwnershipAcrossRolesAndShred(t *testing.T) {
	ctx := context.Background()
	fixture := newEpochScopeFixture(t)
	first, sibling := fixture.open(t), fixture.open(t)
	planet := tag.UID{31, 1}
	epoch := tag.UID{41, 1}
	channelA := safe.ChannelScope(planet, tag.UID{51, 1})
	channelB := safe.ChannelScope(planet, tag.UID{52, 1})
	if err := first.PutKey(ctx, channelA, scopeKey(epoch, safe.KeyRole_ContentKey, 1)); err != nil {
		t.Fatal(err)
	}
	resolved, err := sibling.ResolveChannelScope(planet, epoch)
	if err != nil || resolved != channelA {
		t.Fatalf("unambiguous sibling ownership: %v", err)
	}
	// Different roles must NOT make two distinct containers look unambiguous.
	if err := sibling.PutKey(ctx, channelB, scopeKey(epoch, safe.KeyRole_WriteSeed, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := first.ResolveChannelScope(planet, epoch); !status.IsError(err, status.Code_AuthFailed) {
		t.Fatalf("split-role ownership accepted: %v", err)
	}
	if err := first.ShredKeys(ctx, channelA, []tag.UID{epoch}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.open(t).ResolveChannelScope(planet, epoch); !status.IsError(err, status.Code_AuthFailed) {
		t.Fatalf("shred erased ambiguity: %v", err)
	}
	// A later sibling save must preserve both ownership claims.
	if err := sibling.PutKey(ctx, safe.Scope(tag.UID{32, 1}), scopeKey(tag.UID{42, 1}, safe.KeyRole_ContentKey, 3)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.open(t).ResolveChannelScope(planet, epoch); !status.IsError(err, status.Code_AuthFailed) {
		t.Fatalf("sibling save erased ambiguity: %v", err)
	}
}

func TestEpochKeys_ChannelRejectsPlanetEpochAlias(t *testing.T) {
	ctx := context.Background()
	fixture := newEpochScopeFixture(t)
	store := fixture.open(t)
	planet := tag.UID{61, 1}
	epoch := tag.UID{62, 1}
	planetScope := safe.Scope(planet)
	channelScope := safe.ChannelScope(planet, tag.UID{63, 1})
	if err := store.PutKey(ctx, channelScope, scopeKey(epoch, safe.KeyRole_ContentKey, 1)); err != nil {
		t.Fatal(err)
	}
	if err := store.PutKey(ctx, planetScope, scopeKey(epoch, safe.KeyRole_ContentKey, 2)); err != nil {
		t.Fatal(err)
	}
	if err := store.ShredKeys(ctx, planetScope, []tag.UID{epoch}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.open(t).ResolveChannelScope(planet, epoch); !status.IsError(err, status.Code_AuthFailed) {
		t.Fatalf("shredded planet epoch became channel alias: %v", err)
	}
}

func TestEpochKeys_ConcurrentInstallAndSiblingUnion(t *testing.T) {
	ctx := context.Background()
	fixture := newEpochScopeFixture(t)
	stores := []safe.EpochKeyStore{fixture.open(t), fixture.open(t)}
	scope := safe.Scope(tag.UID{71, 1})
	epoch := tag.UID{72, 1}
	type result struct {
		index     int
		installed bool
		err       error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for index, store := range stores {
		go func(index int, store safe.EpochKeyStore) {
			<-start
			installed, err := store.InstallKey(ctx, scope, scopeKey(epoch, safe.KeyRole_ContentKey, byte(index+1)))
			results <- result{index: index, installed: installed, err: err}
		}(index, store)
	}
	close(start)
	winner := -1
	for range stores {
		outcome := <-results
		if outcome.err == nil {
			if winner >= 0 || !outcome.installed {
				t.Fatal("conflicting installs both succeeded")
			}
			winner = outcome.index
		} else if !status.IsError(outcome.err, status.Code_AuthFailed) {
			t.Fatal(outcome.err)
		}
	}
	if winner < 0 {
		t.Fatal("neither install succeeded")
	}
	assertScopeKey(t, fixture.open(t), scope, scopeKey(epoch, safe.KeyRole_ContentKey, byte(winner+1)))
	// Concurrent unrelated writes must not lose a tuple by whole-tome saving.
	start = make(chan struct{})
	for index, store := range stores {
		go func(index int, store safe.EpochKeyStore) {
			<-start
			err := store.PutKey(ctx, safe.Scope(tag.UID{uint64(80 + index), 1}), scopeKey(epoch, safe.KeyRole_ContentKey, byte(index+10)))
			results <- result{index: index, err: err}
		}(index, store)
	}
	close(start)
	for range stores {
		if outcome := <-results; outcome.err != nil {
			t.Fatal(outcome.err)
		}
	}
	fresh := fixture.open(t)
	for index := range stores {
		assertScopeKey(t, fresh, safe.Scope(tag.UID{uint64(80 + index), 1}), scopeKey(epoch, safe.KeyRole_ContentKey, byte(index+10)))
	}
}

func TestEpochKeys_InstallSaveFailureAndEqualRetry(t *testing.T) {
	ctx := context.Background()
	fixture := newEpochScopeFixture(t)
	backend := &faultTomeStore{inner: safe.NewLocalTomeStore(fixture.path)}
	store, err := safe.OpenEpochKeyStore(ctx, backend, fixture.guard, []byte("scope-test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	scope := safe.Scope(tag.UID{91, 1})
	key := scopeKey(tag.UID{92, 1}, safe.KeyRole_ContentKey, 4)
	backend.failSaves = 1
	if _, err := store.InstallKey(ctx, scope, key); err == nil {
		t.Fatal("failed Save accepted")
	}
	if _, err := store.GetKey(scope, key.EpochID, key.Role); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("failed install published memory: %v", err)
	}
	if installed, err := store.InstallKey(ctx, scope, key); err != nil || !installed {
		t.Fatalf("retry failed: %v", err)
	}
	backend.failSaves = 1
	if _, err := store.InstallKey(ctx, scope, key); err == nil {
		t.Fatal("equal install skipped persistence failure")
	}
	if installed, err := store.InstallKey(ctx, scope, key); err != nil || installed {
		t.Fatalf("equal retry: installed=%v err=%v", installed, err)
	}
	assertScopeKey(t, fixture.open(t), scope, key)
	conflicting := scopeKey(key.EpochID, key.Role, 5)
	if _, err := store.InstallKey(ctx, scope, conflicting); !status.IsError(err, status.Code_AuthFailed) {
		t.Fatalf("conflicting key accepted: %v", err)
	}
	assertScopeKey(t, store, scope, key)
	backend.failSaves = 1
	if err := store.PutKey(ctx, scope, conflicting); err == nil {
		t.Fatal("replacement ignored persistence failure")
	}
	assertScopeKey(t, store, scope, key)
	assertScopeKey(t, fixture.open(t), scope, key)
	if err := store.PutKey(ctx, scope, conflicting); err != nil {
		t.Fatal(err)
	}
	assertScopeKey(t, fixture.open(t), scope, conflicting)
	if _, err := store.InstallKey(ctx, safe.KeyScope{}, key); !status.IsError(err, status.Code_BadRequest) {
		t.Fatalf("nil planet accepted: %v", err)
	}
	if err := store.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstallKey(ctx, scope, key); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("closed install: %v", err)
	}
	if _, err := store.ResolveChannelScope(scope.PlanetID, key.EpochID); !errors.Is(err, safe.ErrStoreClosed) {
		t.Fatalf("closed resolve: %v", err)
	}
}

func TestEpochKeys_EqualInstallPreservesElection(t *testing.T) {
	ctx := context.Background()
	fixture := newEpochScopeFixture(t)
	store := fixture.open(t)
	scope := safe.Scope(tag.UID{101, 1})
	older := scopeKey(tag.UID{102, 1}, safe.KeyRole_ContentKey, 1)
	newer := scopeKey(tag.UID{103, 1}, safe.KeyRole_ContentKey, 2)
	if err := store.PutKey(ctx, scope, older); err != nil {
		t.Fatal(err)
	}
	if err := store.PutKey(ctx, scope, newer); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCurrentEpoch(ctx, scope, older.EpochID); err != nil {
		t.Fatal(err)
	}
	if installed, err := store.InstallKey(ctx, scope, newer); err != nil || installed {
		t.Fatalf("equal install: %v", err)
	}
	current, err := fixture.open(t).GetCurrentKey(scope, safe.KeyRole_ContentKey)
	if err != nil || current.EpochID != older.EpochID {
		t.Fatalf("equal install changed explicit election: %v", err)
	}
	current.Zero()
	if err := store.SetCurrentEpoch(ctx, scope, newer.EpochID); err != nil {
		t.Fatal(err)
	}
	if err := store.ShredKeys(ctx, scope, []tag.UID{newer.EpochID}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.open(t).GetCurrentKey(scope, safe.KeyRole_ContentKey); !status.IsError(err, status.Code_KeyringNotFound) {
		t.Fatalf("reopen silently elected older survivor: %v", err)
	}
}
