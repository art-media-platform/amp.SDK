// Package tag defines amp's identity and addressing primitives: the 16-byte UID (also a fixed-precision timestamp), the
// Name that binds a UTF-8 tag expression to its hash, and the ElementID / Address tuples that locate a value element in
// amp's CRDT schema.
package tag

import (
	"errors"
)

type (

	// UID is a 16-byte universally unique identifier and/or a timestamp with discrete fixed precision: the first 6
	// bytes are UTC whole seconds, the last 10 bytes (80 bits) fractional precision.  UID is big-endian, so UID[0] is
	// most significant.
	UID [2]uint64

	// Name expresses a UTF-8 tag expression and its hash (UID):
	//
	//	Name := {word}("." {word})* [{url_or_identifier}]
	//
	// The name part case-folds to dot-joined words; a URL / identifier part (after the first '/', ':', or '\') stays
	// verbatim — see stdlib/tag/README.md.
	Name struct {
		ID UID // hash of any art-media-platform or other tag expression

		// Optional case-preserved tag expression; canonize(Text) yields the canonic string hashed into ID.  May be
		// dropped (identity is ID alone) so a UID can be matched without disclosing what it names.
		Text string
	}

	// Address addresses a value element in amp's CRDT schema.  A zero EditID means get/set the most appropriate
	// EditID.
	Address struct {
		ElementID     //   00:48   NodeID, AttrID, ItemID
		EditID    UID //   48:64   Midpoint(edit_time, replace_time)
	}

	// ElementID uniquely identifies a value element in amp's CRDT schema.
	ElementID struct {
		NodeID UID //   00:16   host channel; has associated access control list (ACL)
		AttrID UID //   16:32   attribute schema; specifies how ItemID is interpreted
		ItemID UID //   32:48   inline element ID implied by AttrID (e.g. coordinates, timestamp, hash)
	}

	AddressLSM    [AddressLSM_Size]byte          // an Address in its LSM key format
	ElementLSM    [ElementLSM_Size]byte          // a (NodeID, AttrID, ItemID) tuple as a byte array
	EditTable     map[ElementID]UID              // the most recent EditID per ElementID
	OctalDigit    byte                           // base 8 (3 least significant bits)
	OctalEncoding [(UID_Bits + 2) / 3]OctalDigit // octal encoding of a UID
)

const (
	ElementLSM_Size = 3 * UID_Size // NodeID, AttrID, ItemID
	AddressLSM_Size = 4 * UID_Size // NodeID, AttrID, ItemID, EditID

	UID_Size                = 16                   // UID octet size
	UID_Bits                = 8 * UID_Size         // UID bit size
	UID_Base32Length        = (UID_Bits + 4) / 5   // max base32 digit count of a UID
	UID_Base32GroupedLength = UID_Base32Length + 4 // Base32() render length incl. the four 5-5-6-5-5 '-' separators
	UID_LabelLength         = 5                    // AsLabel() digit count: the render's last group
	UID_0_Max               = 0xFFFFFFFFFFFFFFFF   // max allowed value of UID[0] (inclusive)
	UID_1_Max               = 0xFFFFFFFFFFFFFFF0   // max allowed value of UID[1] (inclusive)
	UID_1_Wildcard          = 0xFFFFFFFFFFFFFFFA   // match any UID
	UID_1_Drop              = 0xFFFFFFFFFFFFFFFD   // drop entire attribute (all items) or singular item

	// SeparatorRegex splits a tag expression into component literals on common punctuation and whitespace.
	SeparatorRegex = `[\s\.,!:¿?"&\(\)\[\]\{\}\<\>+-]+`

	// CanonicWildcard is the reserved tag name denoting a wildcard match.
	CanonicWildcard = "*"

	// CanonicSeparator separates literals in a canonic tag string: a '.' marks a tag string at a glance, is compatible
	// with domain names, and is already a familiar scoping character.
	CanonicSeparator     = "."
	CanonicSeparatorChar = byte('.')
)

var (
	ErrUnrecognizedFormat = errors.New("unrecognized ID format")
)
