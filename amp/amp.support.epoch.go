package amp

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

// ── PlanetEpoch: three-layer verbatim-signed authority ────────────────────────
//
// A PlanetEpoch envelope carries two opaque, signed-verbatim byte blobs:
//   Charter — the planet-lifetime-immutable PlanetCharter, marshaled ONCE at
//             genesis and carried byte-identically on every epoch.
//   Terms   — this epoch's EpochTerms, marshaled ONCE when the epoch is minted.
//
// CoSignatures sign the FRAME below; verifiers hash the STORED Charter/Terms
// bytes exactly as received and never re-marshal a PlanetCharter / EpochTerms.
// This removes any dependence on cross-language protobuf-marshal stability — the
// one multi-century footgun of the old proto-deterministic CanonicalBytes.
//
//     FRAME = u64BE(len Charter) || Charter || u64BE(len Terms) || Terms
//
// The length prefixes make the Charter/Terms boundary unforgeable (a byte cannot
// be moved across the split without changing a length); layout evolution is
// signaled by the in-band, signed CharterSchema / TermsSchema fields, so the
// FRAME needs no magic or version tag.

// EpochFrame returns the exact bytes a CoSignature signs and a verifier checks:
// the two verbatim layer blobs, each length-delimited with a big-endian uint64.
// Operates on opaque bytes only — no protobuf library is required to reproduce it.
func EpochFrame(charterBytes, termsBytes []byte) []byte {
	frame := make([]byte, 0, 16+len(charterBytes)+len(termsBytes))
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(charterBytes)))
	frame = append(frame, lenBuf[:]...)
	frame = append(frame, charterBytes...)
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(termsBytes)))
	frame = append(frame, lenBuf[:]...)
	frame = append(frame, termsBytes...)
	return frame
}

// SignedBytes returns the FRAME for this envelope from its STORED Charter/Terms
// bytes.  This is what genesis/rotation signers pass to CryptoKit.Sign and what
// verifiers pass to CryptoKit.Verify — never a re-marshal of the layers.
func (pe *PlanetEpoch) SignedBytes() ([]byte, error) {
	if pe == nil {
		return nil, status.Code_BadRequest.Error("amp: nil PlanetEpoch")
	}
	if len(pe.Charter) == 0 || len(pe.Terms) == 0 {
		return nil, status.Code_BadRequest.Error("amp: PlanetEpoch missing Charter/Terms bytes")
	}
	return EpochFrame(pe.Charter, pe.Terms), nil
}

// ParsedCharter unmarshals the verbatim Charter bytes into a PlanetCharter.
// Reading fields is fine; the AUTHORITY is the stored bytes, never a re-marshal.
func (pe *PlanetEpoch) ParsedCharter() (*PlanetCharter, error) {
	if pe == nil || len(pe.Charter) == 0 {
		return nil, status.Code_BadRequest.Error("amp: PlanetEpoch has no Charter")
	}
	charter := &PlanetCharter{}
	if err := proto.Unmarshal(pe.Charter, charter); err != nil {
		return nil, err
	}
	return charter, nil
}

// IsPublic reports whether this charter's PrivacyMode is Public — signature-only
// with no payload encryption.  Nil-safe; the default (Confidential) reads false.
func (c *PlanetCharter) IsPublic() bool {
	return c != nil && c.Privacy == PrivacyMode_Public
}

// ParsedTerms unmarshals the verbatim Terms bytes into an EpochTerms.
func (pe *PlanetEpoch) ParsedTerms() (*EpochTerms, error) {
	if pe == nil || len(pe.Terms) == 0 {
		return nil, status.Code_BadRequest.Error("amp: PlanetEpoch has no Terms")
	}
	terms := &EpochTerms{}
	if err := proto.Unmarshal(pe.Terms, terms); err != nil {
		return nil, err
	}
	return terms, nil
}

// IsGenesis reports whether these terms describe a founding epoch — one that names no
// predecessor.  A rotation (any later epoch in the chain) carries a non-nil PreviousEpoch.
// The single discriminator the authority gate and the epoch-acceptance state machine share.
// Nil-safe: a nil receiver / nil PreviousEpoch reads as genesis (the getter is nil-safe and
// the `||` short-circuits before UID()).
func (t *EpochTerms) IsGenesis() bool {
	prev := t.GetPreviousEpoch()
	return prev == nil || prev.UID().IsNil()
}

// brandIdentityZero is the shared zero-value instance IdentityOrZero returns when
// a record carries no BrandIdentity.  Read-only by convention — callers never mutate.
var brandIdentityZero = &BrandIdentity{}

// IdentityOrZero returns the mark's BrandIdentity, never nil (a zero-value
// instance when absent) — the nil-safe read path for both Brand and BrandMark.
func (mark *BrandMark) IdentityOrZero() *BrandIdentity {
	if mark == nil || mark.Identity == nil {
		return brandIdentityZero
	}
	return mark.Identity
}

// IdentityOrZero returns the brand's BrandIdentity, never nil (a zero-value
// instance when absent) — the nil-safe read path for both Brand and BrandMark.
func (brand *Brand) IdentityOrZero() *BrandIdentity {
	if brand == nil || brand.Identity == nil {
		return brandIdentityZero
	}
	return brand.Identity
}

// CoSignatureDigest returns the domain-separated digest a founder/admin signs and
// a verifier checks for this epoch — SigningDomain_EpochCoSign (safe.sign.go) bound
// over the FRAME under the epoch's hash policy.  The single authoritative epoch
// cosign digest (the epoch twin of TxSignedDigest): the producer signs it, the
// verifier checks it, and no site re-derives what an epoch co-signature covers.
// The domain tag keeps a founder's epoch signature from being reused as a TxMsg
// author seal, and the fixed 32-byte digest keeps every CryptoKit's input uniform.
func (pe *PlanetEpoch) CoSignatureDigest() ([]byte, error) {
	frame, err := pe.SignedBytes()
	if err != nil {
		return nil, err
	}
	terms, err := pe.ParsedTerms()
	if err != nil {
		return nil, err
	}
	return safe.SigningDigest(terms.EffectiveHashKit(), safe.SigningDomain_EpochCoSign, frame)
}

// VerifyCoSignature checks that cosig is a valid signature over this epoch's
// CoSignatureDigest using signerPubKey under signerKit.  Returns nil on success.
func (pe *PlanetEpoch) VerifyCoSignature(cosig *CoSignature, signerPubKey []byte, signerKit safe.CryptoKitID) error {
	if cosig == nil || len(cosig.Signature) == 0 {
		return status.Code_BadRequest.Error("amp: empty CoSignature")
	}
	digest, err := pe.CoSignatureDigest()
	if err != nil {
		return err
	}
	return safe.VerifySignature(signerKit, cosig.Signature, digest, signerPubKey)
}

// AssembleEpoch marshals charter + terms exactly ONCE, binds the Charter hash
// into Terms (so charter continuity is checkable from the small Terms alone),
// and returns a PlanetEpoch envelope ready for co-signing.  The returned
// envelope's Charter/Terms bytes are the verbatim signed artifact; SignedBytes
// over them yields the FRAME each founder signs.
//
// hashKit selects the digest used for CharterHash (carried, not hardcoded).
// charter.PlanetID / charter.GenesisEpoch and terms.EpochTag must already be set
// by the caller (genesis builder); for genesis, terms.PreviousEpoch is nil and
// terms.EpochHeight is 0.
func AssembleEpoch(charter *PlanetCharter, terms *EpochTerms, hashKit safe.HashKitID) (*PlanetEpoch, error) {
	if charter == nil || terms == nil {
		return nil, status.Code_BadRequest.Error("amp: AssembleEpoch needs charter + terms")
	}
	charterBytes, err := proto.Marshal(charter)
	if err != nil {
		return nil, err
	}
	digest, err := hashBytes(hashKit, charterBytes)
	if err != nil {
		return nil, err
	}
	terms.HashKit = hashKit
	terms.CharterHash = digest
	termsBytes, err := proto.Marshal(terms)
	if err != nil {
		return nil, err
	}
	return &PlanetEpoch{
		Charter:  charterBytes,
		Terms:    termsBytes,
		EpochTag: terms.EpochTag, // advisory denormalized copy (Terms is authoritative)
	}, nil
}

// EpochFromTerms wraps an EpochTerms in a runtime PlanetEpoch envelope carrying
// only its marshaled Terms bytes — no Charter, no signatures.  It is for the
// in-memory session registry, where a node tracks a planet's current epoch
// metadata (kit, label, foyer, vault config) without holding the immutable
// Charter; it is NOT a signable artifact.  ParsedTerms reads the fields back;
// ParsedCharter and SignedBytes are unavailable on the result.  Nil-safe.
func EpochFromTerms(terms *EpochTerms) *PlanetEpoch {
	if terms == nil {
		return &PlanetEpoch{}
	}
	termsBytes, _ := proto.Marshal(terms)
	return &PlanetEpoch{
		EpochTag: terms.EpochTag,
		Terms:    termsBytes,
	}
}

// VerifyCharterContinuity confirms this envelope's carried Charter matches the
// hash its Terms commits to, and (when prev is supplied) that the chain is sound:
// identical Charter bytes, Terms.PreviousEpoch == prev's EpochTag, height + 1,
// and that prev was not Terminal-sealed (no epoch may chain off a Terminal one).
func (pe *PlanetEpoch) VerifyCharterContinuity(prev *PlanetEpoch) error {
	terms, err := pe.ParsedTerms()
	if err != nil {
		return err
	}
	digest, err := hashBytes(terms.HashKit, pe.Charter)
	if err != nil {
		return err
	}
	if !bytes.Equal(digest, terms.CharterHash) {
		return status.Code_AuthFailed.Error("amp: Terms.CharterHash does not match carried Charter")
	}
	if prev == nil {
		return nil // genesis: nothing to chain against
	}
	prevTerms, err := prev.ParsedTerms()
	if err != nil {
		return err
	}
	if prevTerms.Seal == SealState_Sealed {
		return status.Code_AuthFailed.Error("amp: predecessor epoch is Sealed; no successor is valid")
	}
	if !bytes.Equal(pe.Charter, prev.Charter) {
		return status.Code_AuthFailed.Error("amp: Charter bytes differ from predecessor (charter swap)")
	}
	if terms.PreviousEpoch.UID() != prevTerms.EpochTag.UID() {
		return status.Code_AuthFailed.Error("amp: Terms.PreviousEpoch does not point at predecessor")
	}
	if terms.EpochHeight != prevTerms.EpochHeight+1 {
		return status.Code_AuthFailed.Error("amp: EpochHeight is not predecessor + 1")
	}
	// HashKit is stable across a planet's epoch chain: routine rotation carries it
	// forward so a still-in-grace tx authored under the prior epoch verifies under the
	// same digest hash.  A deliberate hash migration is a deferred capability; this is
	// the single site it would relax (and where an authoring-epoch hash resolver lands).
	if terms.HashKit != prevTerms.HashKit {
		return status.Code_AuthFailed.Error("amp: HashKit changed across epoch chain (deliberate hash migration is not yet supported)")
	}
	return nil
}

// hashBytes runs data through the given HashKit and returns the digest.
func hashBytes(kitID safe.HashKitID, data []byte) ([]byte, error) {
	kit, err := safe.NewHashKit(kitID)
	if err != nil {
		return nil, err
	}
	kit.Hasher.Reset()
	kit.Hasher.Write(data)
	return kit.Hasher.Sum(nil), nil
}

// FounderFingerprint returns the 32-byte commitment to a planet's founder
// authority root — the pin a NameServiceRecord / PlanetInvite carries so a
// cold consumer corroborates a first-seen genesis against the founder set it
// was told to expect (SD-channel-governance §8).  Each founder contributes one
// entry — CryptoKitID (16 bytes) ‖ signing pubkey bytes — and entries are
// sorted lexicographically so the commitment is founder-order independent; the
// charter's raw GenesisRequiredSignatures rides as a u32-BE trailer part.
// Digested under the fixed v1 kit via SigningDomain_FounderSet: the consumer
// holds no epoch yet, so the kit cannot be per-planet; a kit migration mints a
// v2 domain and a new wire slot.  Errors on an empty founder set or a founder
// with no key bytes.
func FounderFingerprint(founderKeys map[tag.UID]safe.PubKey, requiredSignatures int32) ([]byte, error) {
	if len(founderKeys) == 0 {
		return nil, status.Code_BadRequest.Error("amp: FounderFingerprint: empty founder set")
	}
	entries := make([][]byte, 0, len(founderKeys))
	for founderID, pub := range founderKeys {
		if len(pub.Bytes) == 0 {
			return nil, status.Code_BadRequest.Errorf("amp: FounderFingerprint: founder %s has no signing key bytes", founderID.AsLabel())
		}
		entry := pub.CryptoKitID.AppendTo(make([]byte, 0, 16+len(pub.Bytes)))
		entry = append(entry, pub.Bytes...)
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i], entries[j]) < 0
	})
	quorum := make([]byte, 4)
	binary.BigEndian.PutUint32(quorum, uint32(requiredSignatures))
	parts := append(entries, quorum)
	return safe.SigningDigest(safe.HashKitID_Blake2s_256, safe.SigningDomain_FounderSet, parts...)
}
