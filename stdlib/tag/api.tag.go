package tag

type (

	// UID is a 16-byte universally unique identifier or a timestamp with discrete fixed precision.
	//
	// When used as a timestamp, the first 6 bytes are UTC whole seconds followed by 10 bytes (80 bits) of fractional precision.
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
		ChanID UID //   00:16   Channel ID
		AttrID UID //   16:32   Any attribute schema UID or hash; specifies how ItemID is interpreted.
		ItemID UID //   32:48   Any client-defined UID or inline value (e.g. a hash, coordinates, etc)
		EditID UID //   48:64   (edit.TimeID + replacing.TimeID) / 2
	}

	// AddressLSM is an Address serialized into its corresponding LSM key format.
	AddressLSM [AddressLength]byte

	// AddressID is AddressLSM sans EditID.
	AddressID [AddressIDLength]byte

	// EditTable tracks the most recent EditID for a given AddressID.
	EditTable map[AddressID]UID

	// AddressRange expresses an ordered range of Addresses.
	AddressRange struct {
		Lo     Address // inclusive lower bound
		Hi     Address // inclusive upper bound
		Weight float32 // > 0 includes, < 0 excludes, 0 no-op
	}

	OctalDigit    byte           // base 8 (3 least significant bits)
	OctalEncoding [64]OctalDigit // octal encoding of a UID
)

const (
	// Byte length of a serialized Address / AddressLSM.
	AddressLength = AddressIDLength + 16
	// Byte length of a serialized AddressID (an Address sans EditID).
	AddressIDLength = (8 + 16) + 16 + (8 + 16)

	PackageTags      = "amp.spec.tag.v0.702"
	UID_Base32Length = (8*16 + 4) / 5
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
