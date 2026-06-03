package safe

import (
	"github.com/art-media-platform/amp.SDK/stdlib/status"
)

// Registry is an init()-only lookup table keyed by K, holding *V descriptors.
// It is written only by Register during package init() and is read-only once
// main() starts, so it needs no lock: registration happens-before every lookup,
// a contract go test -race enforces.
//
// Open naming is not open trust.  Get fails closed for any unregistered key, and
// the trusted set is pinned in signed EpochTerms — never widened by whatever a
// process happens to have registered (or blank-imported) here.
//
// Values are held by pointer on purpose: every caller shares one instance (the
// func tables are never copied), and *V stays comparable even when V is not — a
// HashSpec carries a func field, so only its pointer has an identity Register can
// dedup by.
type Registry[K comparable, V any] struct {
	label string // descriptor kind, woven into messages ("CryptoKit", "HashKit")
	byID  map[K]*V
}

// newRegistry returns an empty Registry that labels its descriptors in messages.
func newRegistry[K comparable, V any](label string) Registry[K, V] {
	return Registry[K, V]{
		label: label,
		byID:  map[K]*V{},
	}
}

// Register inserts val under key.  It must be called from init().  Re-registering
// the same pointer is a no-op; a different *V under an in-use key is an error.
func (reg *Registry[K, V]) Register(key K, val *V) error {
	if existing := reg.byID[key]; existing != nil && existing != val {
		return status.Code_AlreadyRegistered.Errorf("%s %v is already registered", reg.label, key)
	}
	reg.byID[key] = val
	return nil
}

// Get returns the *V registered under key, failing closed if none is.
func (reg *Registry[K, V]) Get(key K) (*V, error) {
	if val := reg.byID[key]; val != nil {
		return val, nil
	}
	return nil, status.Code_ItemNotFound.Errorf("unrecognized %s %v", reg.label, key)
}

// The two orthogonal kit axes: a signing/encryption suite (open namespace) and a
// content-integrity hash (small curated set).  Distinct registries because a hash
// has no keypair and no tie to the suite — bundling them is the TLS-1.2
// cipher-suite-explosion mistake.
var (
	gCryptoKits = newRegistry[CryptoKitID, Kit]("CryptoKit")
	gHashKits   = newRegistry[HashKitID, HashSpec]("HashKit")
)

// RegisterCryptoKit registers a kit's CryptoKit so it can be retrieved via CryptoKit.
// It must be called from init().
func RegisterCryptoKit(kit *Kit) error { return gCryptoKits.Register(kit.ID, kit) }

// CryptoKit fetches a registered kit's CryptoKit, failing closed if unregistered.
func CryptoKit(cryptoKitID CryptoKitID) (*Kit, error) { return gCryptoKits.Get(cryptoKitID) }

// RegisterHashKit registers spec so it can be retrieved via GetHashKit / NewHashKit.
// It must be called from init().
func RegisterHashKit(spec *HashSpec) error { return gHashKits.Register(spec.ID, spec) }

// GetHashKit fetches a registered HashSpec, failing closed if the kit's package
// was never imported.
func GetHashKit(hashKitID HashKitID) (*HashSpec, error) { return gHashKits.Get(hashKitID) }
