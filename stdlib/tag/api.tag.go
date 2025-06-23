package tag

type (

	// UID is a 16-byte universally unique identifier or a timestamp with discrete fixed precision.
	//
	// When used as a timestamp, the first 6 bytes are UTC whole seconds followed by 10 bytes (80 bits) of fractional precision.
	//
	// UID is a big-endian value, so UID[0] is most significant, etc.
	UID [2]uint64

	// Expr expresses a set of UTF-8 literals and its corresponding hash (UID).
	//
	//	tag.Expr := "[{TagOp}*[{utf8_token}]*"
	Expr struct {
		ID      UID    // hash of any art-media-platform or other tag expression
		Canonic string // optional UTF-8 canonic tag expression that generates Expr.ID
	}

	// Addresses a value element in amp's CRDT schema.
	//
	// When EditID is zero, this signals to get/set the most appropriate EditID.
	Address struct {
		NodeID UID //   00:16   host channel; has associated access control list (ACL)
		AttrID UID //   16:32   attribute schema; specifies how ItemID is interpreted
		ItemID UID //   32:48   inline element ID implied by AttrID (e.g. coordinates, timestamp, hash)
		EditID UID //   48:64   (edit_time + replace_time) / 2
		FromID UID //   64:80   entry author ID; source tx loaded via (Node, FromID, EditID)
	}

	// AddressLSM is an Address serialized into its corresponding LSM key format.
	AddressLSM [AddressLSM_Size]byte

	// ElementLSM is a (NodeID, AttrID, ItemID) tuple in the form of a byte array.
	ElementLSM [ElementLSM_Size]byte

	// EditTable tracks the most recent EditID for a given ElementID.
	EditTable map[ElementLSM]UID

	OctalDigit    byte           // base 8 (3 least significant bits)
	OctalEncoding [64]OctalDigit // octal encoding of a UID
)

const (
	ElementLSM_Size = 3 * 16                 // sizeof(NodeID + AttrID + ItemID)
	AddressLSM_Size = ElementLSM_Size + 2*16 // sizeof(ElementID) + sizeof(EditID + FromID)

	PackageTags      = "amp.spec.tag.v0.703"
	UID_Size         = 16                 // UID byte size
	UID_Base32Length = (8*16 + 4) / 5     // max base32 encoding length of a UID
	UID_0_Max        = 0xFFFFFFFFFFFFFFFF // max allowed value of UID[0]
	UID_1_Max        = 0xFFFFFFFFFFFFFFF9 // max allowed value of UID[1]
	UID_1_Wildcard   = 0xFFFFFFFFFFFFFFFA // causes any UID to match

	WithOperators   = `[\.+\s,!?]+`  // commutative (symmetric) binary delimiters
	ThenOperators   = `[\-/\\~:^@]+` // non-commutative binary or unary delimiters
	GroupDelimiters = `[]()<>{}¿?¡!` // TODO: group delimiter pairs

	// The "with" delimiter can be thought of as ADD or SUM and combines two terms in a commutative way like addition.
	// A '.' by convention helps visually identify a tag string, it's compatible with domain names, and is already a familiar scoping character.
	CanonicWith     = "."
	CanonicWithChar = byte('.')

	CanonicThen     = "-"
	CanonicThenChar = byte('-')

	GenesisEditClearBits = uint64(0xFFFFFF)
)
