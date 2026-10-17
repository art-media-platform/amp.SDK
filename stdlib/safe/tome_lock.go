package safe

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// TomeStoreIdentity is optional for stores whose separately constructed handles
// share storage. Wrappers should forward the identity of their underlying store.
// Without it, coordination covers handles using the same comparable TomeStore.
type TomeStoreIdentity interface {
	TomeStoreIdentity() string
}

// tomeLock is the coordination point every live handle on one tome identity
// shares — Enclave and EpochKeyStore handles alike; process-local, not
// cross-process locking.  mu serializes each handle's reload, mutation and
// save; revision counts save attempts, so a handle whose view predates a
// sibling's save rebuilds before it reads.  The point dies with the last
// handle.
type tomeLock struct {
	mu       sync.Mutex
	refs     int // guarded by tomeLocks
	revision atomic.Uint64

	// shredded (epoch tomes): destruction a failed save left pending — any
	// handle's save persists it, and every handle's load applies it.
	shredded map[epochAddress]struct{}

	// removed (enclave tomes): records a handle removed — no handle's reload
	// restores one, and a re-install lifts it.
	removed map[recordAddress]struct{}
}

// recordAddress names one Enclave record: keyring, type and full public key.
type recordAddress struct {
	keyring tag.UID
	keyType KeyType
	pubKey  string
}

func addressOf(rec *KeyPairRecord) recordAddress {
	return recordAddress{
		keyring: recordKeyringID(rec),
		keyType: rec.KeyType,
		pubKey:  string(rec.PubKey),
	}
}

var tomeLocks = struct {
	sync.Mutex
	byID map[any]*tomeLock
}{byID: make(map[any]*tomeLock)}

// acquireTome joins store's coordination point, creating it for the first
// handle.  A store needs a stable identity: its TomeStoreIdentity, or the
// store value itself when comparable.
func acquireTome(store TomeStore) (any, *tomeLock, error) {
	identity := any(store)
	if named, ok := store.(TomeStoreIdentity); ok {
		identity = named.TomeStoreIdentity()
		if identity == "" {
			return nil, nil, fmt.Errorf("safe: empty tome identity")
		}
	} else if store == nil || !reflect.TypeOf(store).Comparable() {
		return nil, nil, fmt.Errorf("safe: tome requires a stable comparable identity")
	}
	tomeLocks.Lock()
	defer tomeLocks.Unlock()
	shared := tomeLocks.byID[identity]
	if shared == nil {
		shared = &tomeLock{
			shredded: make(map[epochAddress]struct{}),
			removed:  make(map[recordAddress]struct{}),
		}
		tomeLocks.byID[identity] = shared
	}
	shared.refs++
	return identity, shared, nil
}

// releaseTome leaves identity's coordination point.
func releaseTome(identity any) {
	tomeLocks.Lock()
	defer tomeLocks.Unlock()
	shared := tomeLocks.byID[identity]
	shared.refs--
	if shared.refs == 0 {
		delete(tomeLocks.byID, identity)
	}
}
