package amp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
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
// This removes any dependence on cross-language protobuf-marshal stability,
// which protobuf does not guarantee.
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

// ContentSealedSince returns the TxTimeID from which this planet refuses
// planet-public content txs: the Terms marker when the carried Charter is
// Confidential, else nil (AOM SD-security-sync.md §7.7).  An envelope with no
// Charter (a session-registry stub, EpochFromTerms) answers nil.
func (pe *PlanetEpoch) ContentSealedSince() tag.UID {
	charter, err := pe.ParsedCharter()
	if err != nil || charter.Privacy != PrivacyMode_Confidential {
		return tag.UID{}
	}
	terms, err := pe.ParsedTerms()
	if err != nil {
		return tag.UID{}
	}
	return terms.GetContentSealedSince().UID()
}

// IsGenesis reports whether these terms describe a founding epoch — one that names no
// predecessor.  A rotation (any later epoch in the chain) carries a non-nil PreviousEpoch.
// The single discriminator the authority gate and the epoch-acceptance state machine share.
// Nil-safe: a nil receiver / nil PreviousEpoch reads as genesis (the getter is nil-safe and
// the `||` short-circuits before UID()).
func (terms *EpochTerms) IsGenesis() bool {
	prev := terms.GetPreviousEpoch()
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

// IdentityOrZero returns the brand's BrandIdentity — the Brand twin of
// BrandMark.IdentityOrZero, with the same nil-safe contract.
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

// AssembleEpoch builds an epoch from a parsed Charter: it marshals charter +
// terms exactly ONCE, binds the Charter hash into Terms (so charter continuity
// is checkable from the small Terms alone), and returns a PlanetEpoch envelope
// ready for co-signing.  The returned envelope's Charter/Terms bytes are the
// verbatim signed artifact; SignedBytes over them yields the FRAME each
// founder signs.
//
// AssembleEpoch is the genesis builder: hashKit selects the digest used for
// CharterHash (carried, not hardcoded); charter.PlanetID /
// charter.GenesisEpoch and terms.EpochTag must already be set, with
// terms.PreviousEpoch nil and terms.EpochHeight 0.  A successor carries its
// predecessor's STORED Charter bytes instead (AssembleSuccessor).
func AssembleEpoch(charter *PlanetCharter, terms *EpochTerms, hashKit safe.HashKitID) (*PlanetEpoch, error) {
	if charter == nil || terms == nil {
		return nil, status.Code_BadRequest.Error("amp: AssembleEpoch needs charter + terms")
	}
	charterBytes, err := proto.Marshal(charter)
	if err != nil {
		return nil, err
	}
	return assembleLayers(charterBytes, terms, hashKit)
}

// AssembleSuccessor assembles the successor of prev from its Terms (built by
// NextEpochTerms).  The Charter is a copy of prev's STORED bytes, never a
// re-marshal of the parsed Charter, so a Charter stored in any valid field
// order keeps its byte identity (SD-canonization-spec §2.2.1); CharterHash is
// their digest under prev's HashKit, which a chain keeps
// (VerifyCharterContinuity).
func AssembleSuccessor(prev *PlanetEpoch, terms *EpochTerms) (*PlanetEpoch, error) {
	if terms == nil || len(prev.GetCharter()) == 0 {
		return nil, status.Code_BadRequest.Error("amp: AssembleSuccessor needs a predecessor with a Charter + terms")
	}
	prevTerms, err := prev.ParsedTerms()
	if err != nil {
		return nil, err
	}
	return assembleLayers(bytes.Clone(prev.Charter), terms, prevTerms.EffectiveHashKit())
}

// assembleLayers binds charterBytes into terms (HashKit, and CharterHash under
// it), marshals terms once, and returns the unsigned envelope — the one
// assembly site under AssembleEpoch and AssembleSuccessor.
func assembleLayers(charterBytes []byte, terms *EpochTerms, hashKit safe.HashKitID) (*PlanetEpoch, error) {
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
		EpochTag: terms.EpochTag, // unsigned copy; Terms is authoritative
	}, nil
}

// ── Successor Terms: the banded clone carry (SD-security-sync §4.4) ──────────
//
// Every epoch's Terms derive through NextEpochTerms: a node's rotation, the
// client's check of a frame it co-signs (VerifySuccessor), and genesis (no
// predecessor).  The predecessor is cloned, so a field this binary does not
// know survives the clone; each top-level field then resolves by its class —
// fields 1–34 by termsFieldTable, every other number by its band
// (ZO-design-conventions §2.8).

// TermsChange is what one epoch changes over its predecessor's Terms.
type TermsChange struct {
	// Template holds the terms this epoch sets.  Without Paths, a set
	// top-level term replaces the predecessor's and a zero term carries.
	Template *EpochTerms

	// Paths names the terms this epoch sets or clears, each a dotted proto
	// field path ("Label", "VaultConfig.VaultAddrs"): a path the Template
	// holds a value at sets it, a path it holds none at clears it, and a
	// repeated or message value replaces whole.  Every term the Template
	// sets must be named.
	Paths []string

	// EpochTag is the new epoch's own identity.
	EpochTag *Tag

	// IntroduceSealedSince sets ContentSealedSince to EpochTag when the
	// predecessor carries none: a Confidential genesis, or the one rotation
	// that introduces the marker (SD-security-sync §7.7).
	IntroduceSealedSince bool
}

// termsClass is how a successor derives one top-level EpochTerms field.
type termsClass int32

const (
	termsChain           termsClass = iota + 1 // the chain position
	termsAssembled                             // set by assembly
	termsKit                                   // template's suite pair, or carried
	termsCarry                                 // carried; a template sets it
	termsCarryOnly                             // carried; no template sets it
	termsTemplateOnly                          // the template's, never carried
	termsEpochLocal                            // 500–999: the template's, dropped
	termsPredecessorOnly                       // carried; IntroduceSealedSince
	termsMustUnderstand                        // derived, else refused
	termsUnallocated                           // refused
)

// EpochTerms field-number band edges (ZO-design-conventions §2.8).
const (
	termsBandEpochLocal     = 500  // 500–999: epoch-local, dropped
	termsBandMustUnderstand = 1000 // 1000–1999: must-understand
	termsBandUnallocated    = 2000 // ≥ 2000: unallocated
)

// termsFieldTable classifies EpochTerms fields 1–34 by NUMBER; the name rides
// in the comment, so a field rename touches no code here.
var termsFieldTable = map[protoreflect.FieldNumber]termsClass{
	1:  termsChain,           // TermsSchema
	2:  termsChain,           // EpochTag
	3:  termsChain,           // PreviousEpoch
	4:  termsChain,           // EpochHeight
	5:  termsAssembled,       // HashKit
	6:  termsAssembled,       // CharterHash
	7:  termsKit,             // CryptoKitID_0
	8:  termsKit,             // CryptoKitID_1
	10: termsCarry,           // Label
	11: termsCarry,           // Mark
	12: termsCarry,           // Foyer
	13: termsCarry,           // Index
	16: termsCarry,           // GovernanceGroup
	18: termsCarry,           // RequiredSignatures
	20: termsCarry,           // Seal
	21: termsCarry,           // Admission
	22: termsTemplateOnly,    // CodexEdition
	25: termsCarry,           // VaultConfig
	32: termsCarry,           // MaxGracePeriod
	33: termsCarryOnly,       // LicenseClass
	34: termsPredecessorOnly, // ContentSealedSince
}

// termsDerivation sets one must-understand field on a successor.  It owns
// the field's value: a no-path template's value for the field is the
// derivation's to read (change.Template), and a path naming it refuses.
type termsDerivation func(prev, next *EpochTerms, change TermsChange) error

// termsDerivations holds the derivation of every known must-understand field
// (1000–1999); the band test holds each such field to an entry.
var termsDerivations = map[protoreflect.FieldNumber]termsDerivation{}

// termsFieldClass classifies an EpochTerms field number, known or not: the
// 1–34 table, else the band.
func termsFieldClass(num protoreflect.FieldNumber) termsClass {
	if class, listed := termsFieldTable[num]; listed {
		return class
	}
	switch {
	case num < termsBandEpochLocal:
		return termsCarry
	case num < termsBandMustUnderstand:
		return termsEpochLocal
	case num < termsBandUnallocated:
		return termsMustUnderstand
	}
	return termsUnallocated
}

// ErrTermsMustUnderstand wraps a refusal to author a successor of an epoch
// carrying a field this binary must understand and cannot derive (ZO §2.8).
// Vaults still admit such an epoch and readers still read it.
var ErrTermsMustUnderstand = status.Code_Unimplemented.Error("amp: EpochTerms field must be understood to author a successor")

// ErrTermsTemplate wraps a refused terms template or path: a field this binary
// does not know, a term no template sets, or a path outside the grammar.
var ErrTermsTemplate = status.Code_BadValue.Error("amp: EpochTerms template refused")

// NextEpochTerms derives an epoch's Terms from its predecessor's (nil for a
// genesis: no PreviousEpoch, height 0) and change — the one derivation site
// for every epoch (SD-security-sync §4.4).  It clones prev and advances the
// chain fields; resolves the clone's fields by class — an unknown field by
// its band: 1–499 carried, 500–999 dropped, ≥ 1000 refused naming its number;
// applies the template (TermsChange); and runs each must-understand field's
// derivation.  HashKit and CharterHash are left to assembly
// (AssembleSuccessor).  A template carrying a field this binary does not
// know, at any depth, is refused.
func NextEpochTerms(prev *EpochTerms, change TermsChange) (*EpochTerms, error) {
	if change.EpochTag.UID().IsNil() {
		return nil, status.Code_BadRequest.Error("amp: NextEpochTerms needs the new EpochTag")
	}
	if prev != nil && change.EpochTag.UID() == prev.GetEpochTag().UID() {
		return nil, status.Code_BadRequest.Error("amp: NextEpochTerms: the new EpochTag is the predecessor's own")
	}
	template := &EpochTerms{}
	if change.Template != nil {
		template = proto.Clone(change.Template).(*EpochTerms)
	}
	if err := refuseUnknownFields(template.ProtoReflect(), ""); err != nil {
		return nil, err
	}
	next := &EpochTerms{}
	if prev != nil {
		next = proto.Clone(prev).(*EpochTerms)
		next.PreviousEpoch = next.EpochTag
		next.EpochHeight = prev.GetEpochHeight() + 1
	}
	next.TermsSchema = 1
	next.EpochTag = proto.Clone(change.EpochTag).(*Tag)
	if err := applyTermsBands(next); err != nil {
		return nil, err
	}
	applyTemplate := applyTermsTemplate
	if len(change.Paths) > 0 {
		applyTemplate = func(next, template *EpochTerms) error {
			return applyTermsPaths(next, template, change.Paths)
		}
	}
	if err := applyTemplate(next, template); err != nil {
		return nil, err
	}
	if change.IntroduceSealedSince && next.GetContentSealedSince().UID().IsNil() {
		next.ContentSealedSince = proto.Clone(change.EpochTag).(*Tag)
	}
	if err := applyTermsDerivations(prev, next, change); err != nil {
		return nil, err
	}
	return next, nil
}

// applyTermsBands resolves the clone's fields by class before the template
// applies: an unknown field carries (1–499), drops (500–999) or refuses
// (≥ 1000); a known template-only or epoch-local field clears; a known field
// ≥ 2000 refuses.
func applyTermsBands(next *EpochTerms) error {
	msg := next.ProtoReflect()
	raw := msg.GetUnknown()
	kept := protoreflect.RawFields(nil)
	for len(raw) > 0 {
		num, _, size := protowire.ConsumeField(raw)
		if size < 0 {
			return status.Code_DataFailure.Errorf("amp: predecessor EpochTerms carries a malformed unknown field: %v", protowire.ParseError(size))
		}
		switch termsFieldClass(num) {
		case termsCarry:
			kept = append(kept, raw[:size]...)
		case termsEpochLocal:
		default:
			return fmt.Errorf("%w: the predecessor carries field %d, unknown to this binary", ErrTermsMustUnderstand, num)
		}
		raw = raw[size:]
	}
	msg.SetUnknown(kept)
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		switch termsFieldClass(fd.Number()) {
		case termsTemplateOnly, termsEpochLocal:
			msg.Clear(fd)
		case termsUnallocated:
			if msg.Has(fd) {
				return fmt.Errorf("%w: the predecessor carries %s (%d), an unallocated number", ErrTermsMustUnderstand, fd.Name(), fd.Number())
			}
		}
	}
	return nil
}

// applyTermsTemplate is the no-path rule: a set top-level term replaces the
// carried one and a zero term carries; the suite pair moves as one; a
// template-only term takes the template's value; chain and assembled fields
// on the template are ignored.  A carry-only or predecessor-only term set on
// the template refuses.
func applyTermsTemplate(next, template *EpochTerms) error {
	dst := next.ProtoReflect()
	err := error(nil)
	template.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch termsFieldClass(fd.Number()) {
		case termsCarry, termsTemplateOnly, termsEpochLocal:
			dst.Set(fd, value)
		case termsCarryOnly, termsPredecessorOnly, termsUnallocated:
			err = fmt.Errorf("%w: the template sets %s (%d), which no template sets", ErrTermsTemplate, fd.Name(), fd.Number())
		}
		return err == nil
	})
	if kit := template.CryptoKitID(); err == nil && !kit.IsNil() {
		next.SetCryptoKitID(kit)
	}
	return err
}

// applyTermsPaths applies the template at each named path: a value sets, an
// absent one clears, a repeated or message leaf replaces whole (never
// proto.Merge, which appends lists and merges messages).  A path resolves by
// proto field names; it may name a carried term or a leaf beneath one, a
// template-only term whole, or both suite halves.  Every term the template
// sets must be named, or the template refuses: never a silent ignore.
func applyTermsPaths(next, template *EpochTerms, paths []string) error {
	resolved := make([][]protoreflect.FieldDescriptor, 0, len(paths))
	named := make(map[string]bool, len(paths))
	kitHalves := make(map[protoreflect.FieldNumber]bool, 2)
	for _, path := range paths {
		fields, err := resolveTermsPath(path)
		if err != nil {
			return err
		}
		top := fields[0]
		switch termsFieldClass(top.Number()) {
		case termsCarry:
		case termsKit:
			kitHalves[top.Number()] = true
		case termsTemplateOnly, termsEpochLocal:
			if len(fields) > 1 {
				return fmt.Errorf("%w: path %q: %s is set whole", ErrTermsTemplate, path, top.Name())
			}
		default:
			return fmt.Errorf("%w: path %q: %s (%d) is not settable", ErrTermsTemplate, path, top.Name(), top.Number())
		}
		resolved = append(resolved, fields)
		named[path] = true
	}
	if len(kitHalves) == 1 {
		return fmt.Errorf("%w: a suite path must name both CryptoKitID halves", ErrTermsTemplate)
	}
	if err := refuseUnnamedFields(template.ProtoReflect(), "", named); err != nil {
		return err
	}
	for _, fields := range resolved {
		applyTermsPath(next.ProtoReflect(), template.ProtoReflect(), fields)
	}
	return nil
}

// resolveTermsPath resolves a dotted EpochTerms field path; a segment this
// binary does not know, or a step through a repeated field, refuses.
func resolveTermsPath(path string) ([]protoreflect.FieldDescriptor, error) {
	desc := (&EpochTerms{}).ProtoReflect().Descriptor()
	segments := strings.Split(path, ".")
	fields := make([]protoreflect.FieldDescriptor, 0, len(segments))
	for i, segment := range segments {
		fd := desc.Fields().ByName(protoreflect.Name(segment))
		if fd == nil {
			return nil, fmt.Errorf("%w: path %q: %q is not a field this binary knows", ErrTermsTemplate, path, segment)
		}
		fields = append(fields, fd)
		if i == len(segments)-1 {
			break
		}
		if fd.Message() == nil || fd.IsList() || fd.IsMap() {
			return nil, fmt.Errorf("%w: path %q: %s is not a singular message", ErrTermsTemplate, path, segment)
		}
		desc = fd.Message()
	}
	return fields, nil
}

// applyTermsPath sets or clears one resolved path's leaf on dst from src,
// descending only where either side holds the parent.
func applyTermsPath(dst, src protoreflect.Message, fields []protoreflect.FieldDescriptor) {
	fd := fields[0]
	if len(fields) == 1 {
		if src.Has(fd) {
			dst.Set(fd, src.Get(fd))
		} else {
			dst.Clear(fd)
		}
		return
	}
	if !src.Has(fd) && !dst.Has(fd) {
		return
	}
	applyTermsPath(dst.Mutable(fd).Message(), src.Get(fd).Message(), fields[1:])
}

// refuseUnnamedFields refuses a template term no path names: each set field
// at or beneath prefix must be named by a path, itself or a parent; a set
// message holding no field must be named itself.
func refuseUnnamedFields(msg protoreflect.Message, prefix string, named map[string]bool) error {
	err := error(nil)
	msg.Range(func(fd protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		path := prefix + string(fd.Name())
		populated := false
		if fd.Message() != nil && !fd.IsList() && !fd.IsMap() {
			value.Message().Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool {
				populated = true
				return false
			})
		}
		switch {
		case named[path]:
		case populated:
			err = refuseUnnamedFields(value.Message(), path+".", named)
		default:
			err = fmt.Errorf("%w: the template sets %s, which no path names", ErrTermsTemplate, path)
		}
		return err == nil
	})
	return err
}

// refuseUnknownFields refuses a message carrying, at any depth, a field this
// binary does not know: a template term it could neither apply nor check.
func refuseUnknownFields(msg protoreflect.Message, prefix string) error {
	if raw := msg.GetUnknown(); len(raw) > 0 {
		nums := []protoreflect.FieldNumber{}
		for len(raw) > 0 {
			num, _, size := protowire.ConsumeField(raw)
			if size < 0 {
				break
			}
			nums = append(nums, num)
			raw = raw[size:]
		}
		return fmt.Errorf("%w: the template sets %sfield(s) %v, unknown to this binary", ErrTermsTemplate, prefix, nums)
	}
	err := error(nil)
	msg.Range(func(fd protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if fd.Message() == nil || fd.IsMap() {
			return true
		}
		inner := prefix + string(fd.Name()) + "."
		if !fd.IsList() {
			err = refuseUnknownFields(value.Message(), inner)
			return err == nil
		}
		list := value.List()
		for i := 0; i < list.Len() && err == nil; i++ {
			err = refuseUnknownFields(list.Get(i).Message(), inner)
		}
		return err == nil
	})
	return err
}

// applyTermsDerivations runs each known must-understand field's derivation;
// a field without one refuses.
func applyTermsDerivations(prev, next *EpochTerms, change TermsChange) error {
	fields := next.ProtoReflect().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if termsFieldClass(fd.Number()) != termsMustUnderstand {
			continue
		}
		derive, registered := termsDerivations[fd.Number()]
		if !registered {
			return fmt.Errorf("%w: %s (%d) has no derivation", ErrTermsMustUnderstand, fd.Name(), fd.Number())
		}
		if err := derive(prev, next, change); err != nil {
			return err
		}
	}
	return nil
}

// VerifySuccessor checks that pe is the successor of prev that change
// derives: VerifyCharterContinuity(prev), then pe's Terms proto.Equal to
// NextEpochTerms over prev's, fed pe's own EpochTag and assembled by
// AssembleSuccessor — never byte equality, which differs when one side knows
// a field the other does not.  The one successor check, for a client's frame
// check before it co-signs and a node's refusal before it commits
// (SD-security-sync §4.4).
func (pe *PlanetEpoch) VerifySuccessor(prev *PlanetEpoch, change TermsChange) error {
	if prev == nil {
		return status.Code_BadRequest.Error("amp: VerifySuccessor needs the predecessor")
	}
	if err := pe.VerifyCharterContinuity(prev); err != nil {
		return err
	}
	terms, err := pe.ParsedTerms()
	if err != nil {
		return err
	}
	prevTerms, err := prev.ParsedTerms()
	if err != nil {
		return err
	}
	change.EpochTag = terms.GetEpochTag()
	derived, err := NextEpochTerms(prevTerms, change)
	if err != nil {
		return err
	}
	expected, err := AssembleSuccessor(prev, derived)
	if err != nil {
		return err
	}
	want, err := expected.ParsedTerms()
	if err != nil {
		return err
	}
	if !proto.Equal(terms, want) {
		return status.Code_AuthFailed.Errorf("amp: successor Terms differ from their derivation at %v", termsDiff(terms, want))
	}
	return nil
}

// termsDiff names the top-level EpochTerms fields where got and want differ;
// "unknown" when their unknown fields do.
func termsDiff(got, want *EpochTerms) []string {
	diffs := []string{}
	gotMsg := got.ProtoReflect()
	wantMsg := want.ProtoReflect()
	fields := gotMsg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if gotMsg.Has(fd) != wantMsg.Has(fd) || !gotMsg.Get(fd).Equal(wantMsg.Get(fd)) {
			diffs = append(diffs, string(fd.Name()))
		}
	}
	gotUnknown := &EpochTerms{}
	gotUnknown.ProtoReflect().SetUnknown(gotMsg.GetUnknown())
	wantUnknown := &EpochTerms{}
	wantUnknown.ProtoReflect().SetUnknown(wantMsg.GetUnknown())
	if !proto.Equal(gotUnknown, wantUnknown) {
		diffs = append(diffs, "unknown")
	}
	return diffs
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

// VerifyEpochTag refuses an envelope whose Terms name no epoch, or whose
// unsigned EpochTag copy differs from the signed Terms.EpochTag; an absent
// copy is a mismatch (SD-canonization-spec.md §2.2.1).
func (pe *PlanetEpoch) VerifyEpochTag() error {
	terms, err := pe.ParsedTerms()
	if err != nil {
		return err
	}
	termsTag := terms.GetEpochTag().UID()
	if termsTag.IsNil() {
		return status.Code_AuthFailed.Error("amp: Terms.EpochTag is not set")
	}
	if pe.GetEpochTag().UID() != termsTag {
		return status.Code_AuthFailed.Error(
			"amp: envelope EpochTag does not match Terms.EpochTag")
	}
	return nil
}

// VerifyCharterContinuity confirms this envelope's EpochTag copy matches its
// Terms (VerifyEpochTag), its carried Charter matches the hash its Terms
// commits to, its ContentSealedSince obeys the chain rule (verifySealedSince),
// and (when prev is supplied) that the chain is sound: identical Charter
// bytes, Terms.PreviousEpoch == prev's EpochTag, height + 1, and that prev
// was not Terminal-sealed (no epoch may chain off a Terminal one).
func (pe *PlanetEpoch) VerifyCharterContinuity(prev *PlanetEpoch) error {
	if err := pe.VerifyEpochTag(); err != nil {
		return err
	}
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
		return pe.verifySealedSince(terms, nil) // genesis: nothing else to chain against
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
	return pe.verifySealedSince(terms, prevTerms)
}

// ErrSealedSinceContinuity wraps every ContentSealedSince chain-rule failure.
// The failure is version-shaped — a binary that predates the marker rotates
// without it — not forgery-shaped, so a verifier refuses such an epoch
// without punishing its signers (AOM SD-security-sync.md §4.4).
var ErrSealedSinceContinuity = status.Code_AuthFailed.Error("amp: ContentSealedSince continuity")

// verifySealedSince holds EpochTerms.ContentSealedSince to its chain rule
// (AOM SD-security-sync.md §4.4, §7.7): a predecessor's marker is carried
// unchanged, and a marker is introduced only on a Confidential Charter,
// equal to the introducing epoch's own EpochTag.  prevTerms is nil at
// genesis.  A failure wraps ErrSealedSinceContinuity.
func (pe *PlanetEpoch) verifySealedSince(terms, prevTerms *EpochTerms) error {
	marker := terms.GetContentSealedSince().UID()
	if prior := prevTerms.GetContentSealedSince().UID(); prior.IsSet() {
		if marker != prior {
			return fmt.Errorf("%w: dropped or changed from the predecessor's", ErrSealedSinceContinuity)
		}
		return nil
	}
	if marker.IsNil() {
		return nil
	}
	if marker != terms.GetEpochTag().UID() {
		return fmt.Errorf("%w: introduced at other than its epoch's own EpochTag", ErrSealedSinceContinuity)
	}
	charter, err := pe.ParsedCharter()
	if err != nil {
		return err
	}
	if charter.Privacy != PrivacyMode_Confidential {
		return fmt.Errorf("%w: introduced on a %v Charter", ErrSealedSinceContinuity, charter.Privacy)
	}
	return nil
}

// SupportedCharterFeatures is every CharterFeature this binary implements;
// a genesis naming another is refused (VerifyCharterFeatures).
var SupportedCharterFeatures = []CharterFeature{
	CharterFeature_FounderAdmission,
}

// EraRequiredFeatures is the feature set every genesis of this era carries
// (both Charter producers stamp it) and every reader of this era requires: a
// Charter omitting one is refused at every VerifyGenesis caller, and the
// device re-founds (SD-security-sync §4.4; the era boundary).
var EraRequiredFeatures = []CharterFeature{
	CharterFeature_FounderAdmission,
}

// VerifyCharterFeatures is the era boundary: charter.RequiredFeatures must
// hold every EraRequiredFeatures entry, name only SupportedCharterFeatures
// entries, and never CharterFeatureUnset.  The list reads as a set
// (duplicates normalize).  The Charter is inside the founder-signed genesis
// frame, so the set cannot be altered without re-signing.
func VerifyCharterFeatures(charter *PlanetCharter) error {
	named := make(map[CharterFeature]struct{}, len(charter.GetRequiredFeatures()))
	for _, feature := range charter.GetRequiredFeatures() {
		if feature == CharterFeature_CharterFeatureUnset {
			return status.Code_AuthFailed.Error(
				"amp: genesis Charter names CharterFeatureUnset")
		}
		if !slices.Contains(SupportedCharterFeatures, feature) {
			return status.Code_AuthFailed.Errorf(
				"amp: genesis Charter requires feature %d this binary lacks", feature)
		}
		named[feature] = struct{}{}
	}
	for _, feature := range EraRequiredFeatures {
		if _, ok := named[feature]; !ok {
			return status.Code_AuthFailed.Errorf(
				"amp: genesis Charter omits this era's required feature %v", feature)
		}
	}
	return nil
}

// VerifyGenesis is the one structural check of a founding epoch: its
// EpochTag and CharterHash (VerifyCharterContinuity with no predecessor),
// Terms naming no predecessor at height 0, a Charter naming planetID and
// this epoch as its genesis (SD-channel-governance §3.3), and the era's
// required feature set (VerifyCharterFeatures).  The founder signatures are
// the caller's to check (acc.VerifyGenesisBrand).
func (pe *PlanetEpoch) VerifyGenesis(planetID tag.UID) error {
	if err := pe.VerifyCharterContinuity(nil); err != nil {
		return err
	}
	terms, err := pe.ParsedTerms()
	if err != nil {
		return err
	}
	if !terms.IsGenesis() || terms.EpochHeight != 0 {
		return status.Code_AuthFailed.Error(
			"amp: not a founding epoch (names a predecessor or height > 0)")
	}
	charter, err := pe.ParsedCharter()
	if err != nil {
		return err
	}
	if planetID.IsNil() || charter.GetPlanetID().UID() != planetID {
		return status.Code_AuthFailed.Error(
			"amp: genesis Charter names another planet")
	}
	if charter.GetGenesisEpoch().UID() != terms.GetEpochTag().UID() {
		return status.Code_AuthFailed.Error(
			"amp: genesis Charter names another epoch as its genesis")
	}
	return VerifyCharterFeatures(charter)
}

// planetIDDomain separates FounderPlanetID's hash from every other UID
// derivation over the same bytes.
const planetIDDomain = "amp.planet-id.v1"

// FounderPlanetID is the PlanetID a founding takes when its request carries a
// nonce: tag.UID_HashLiteral("amp.planet-id.v1" ‖ founderID ‖ nonce), each
// UID as its 16 big-endian bytes (UID.AppendTo) — blake2s-256, folded to 128
// bits.  The planet is bound to its founder, so no founding names a UID it
// did not derive, another member's home planet (home UID = member UID)
// included (DD-name-service.md §5.4).  The one derivation site: the CLI
// derives it to check the challenge it co-signs; the node derives it to found.
func FounderPlanetID(founderID, nonce tag.UID) tag.UID {
	literal := append([]byte(planetIDDomain), founderID.AppendTo(nil)...)
	return tag.UID_HashLiteral(nonce.AppendTo(literal))
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
// was told to expect (AOM SD-channel-governance.md §8).  Each founder contributes one
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

// ── EpochLink: prev-link epoch-key chain ──────────────────────────────────────
//
// An EpochLink publishes ToEpoch's planet ContentKey sealed under a subkey of
// FromEpoch's, so holding any newer key transitively unlocks the archive back
// to the nearest deliberate cut (AOM SD-security-sync.md §5.6).  Seal and open live
// here as the one authoritative pair; both epoch UIDs bind the AEAD as AAD so a
// box cannot be transplanted onto a different link.

// epochLinkPurpose is the domain-separation label an EpochLink subkey derives
// under — distinct from the "content" / "member-proof" tx domains.
const epochLinkPurpose = "epoch-link"

// epochLinkAAD returns FromEpoch ‖ ToEpoch as the box's additional
// authenticated data.  Identity is bytes (UID.AppendTo), never a render.
func epochLinkAAD(fromEpoch, toEpoch tag.UID) []byte {
	aad := fromEpoch.AppendTo(make([]byte, 0, 32))
	return toEpoch.AppendTo(aad)
}

// SealEpochLinkBox seals toKey (an older epoch's ContentKey) under a subkey
// derived from fromKey (a newer epoch's ContentKey), returning the box an
// EpochLink publishes.  The box's EpochID self-describes the enclosed key
// (ToEpoch) and its CryptoKitID stamps the kit the key installs under.
func SealEpochLinkBox(rand io.Reader, fromKey, toKey safe.SymKey) (*safe.EncryptedSymKey, error) {
	if fromKey.EpochID.IsNil() || toKey.EpochID.IsNil() || len(fromKey.Bytes) == 0 || len(toKey.Bytes) == 0 {
		return nil, status.Code_BadRequest.Error("amp: SealEpochLinkBox: both keys must carry EpochID and key bytes")
	}
	if toKey.EpochID.CompareTo(fromKey.EpochID) >= 0 {
		return nil, status.Code_BadRequest.Error("amp: SealEpochLinkBox: ToEpoch must predate FromEpoch")
	}
	subKey, err := safe.DeriveSubKey(fromKey.Bytes, epochLinkPurpose)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(subKey)

	nonce, cipherblob, err := safe.SealAEAD(rand, subKey, toKey.Bytes, epochLinkAAD(fromKey.EpochID, toKey.EpochID))
	if err != nil {
		return nil, err
	}
	return &safe.EncryptedSymKey{
		CryptoKitID_0: toKey.CryptoKitID[0],
		CryptoKitID_1: toKey.CryptoKitID[1],
		EpochID_0:     toKey.EpochID[0],
		EpochID_1:     toKey.EpochID[1],
		Ciphertext:    append(nonce, cipherblob...),
	}, nil
}

// OpenEpochLinkBox unseals link.Box with fromKey (the link's FromEpoch
// ContentKey) and returns the enclosed older epoch's ContentKey, ready for
// EpochKeyStore install.  Caller must Zero the returned key when done.
func OpenEpochLinkBox(fromKey safe.SymKey, link *EpochLink) (safe.SymKey, error) {
	fromEpoch := link.GetFromEpoch().UID()
	toEpoch := link.GetToEpoch().UID()
	box := link.GetBox()
	if box == nil || fromEpoch.IsNil() || toEpoch.IsNil() {
		return safe.SymKey{}, status.Code_BadRequest.Error("amp: OpenEpochLinkBox: malformed link")
	}
	if fromKey.EpochID != fromEpoch {
		return safe.SymKey{}, status.Code_BadRequest.Error("amp: OpenEpochLinkBox: fromKey does not match link.FromEpoch")
	}
	if boxEpoch := (tag.UID{box.EpochID_0, box.EpochID_1}); boxEpoch != toEpoch {
		return safe.SymKey{}, status.Code_BadRequest.Error("amp: OpenEpochLinkBox: box EpochID does not match link.ToEpoch")
	}
	if len(box.Ciphertext) < safe.NonceSize {
		return safe.SymKey{}, status.Code_DecryptFailed.Error("amp: OpenEpochLinkBox: box too short")
	}
	subKey, err := safe.DeriveSubKey(fromKey.Bytes, epochLinkPurpose)
	if err != nil {
		return safe.SymKey{}, err
	}
	defer safe.Zero(subKey)

	plain, err := safe.OpenAEAD(subKey, box.Ciphertext[:safe.NonceSize], box.Ciphertext[safe.NonceSize:], epochLinkAAD(fromEpoch, toEpoch))
	if err != nil {
		return safe.SymKey{}, err
	}
	return safe.SymKey{
		CryptoKitID: safe.CryptoKitID{box.CryptoKitID_0, box.CryptoKitID_1},
		EpochID:     toEpoch,
		Role:        safe.KeyRole_ContentKey,
		Bytes:       plain,
	}, nil
}
