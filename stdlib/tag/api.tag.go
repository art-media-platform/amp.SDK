package tag

type (

	// UID is a 16 byte universally unique identifier (UUID) or a timestamp with fixed precision.
	//
	// When used as a timestamp, the first 6 bytes are UTC whole seconds followed by 12 bytes of fractional precision.
	UID [2]uint64

	// U3D is a 24 byte LSM index that clients use to store 2 or 3 element tuple -- see enum U3DFormat.
	U3D [3]uint64

	// Expr expresses a set of UTF8 literals and its corresponding hash (UID).
	//
	//	tag.Expr := "[{TagOp}*[{utf8_token}]*"
	Expr struct {
		ID      UID    // hash of any art-media-platform or other tag expression
		Canonic string // optional UTF8 canonic tag expression that generates Expr.ID
	}

	// Addresses a value element address in amp's CRDT schema.
	//
	// When EditID is zero, this signals to get/set the most appropriate EditID.
	Address struct {
		ChanID U3D //   00:24   Channel ID
		AttrID UID //   24:40   Any attribute schema UID or hash; specifies how ItemID is interpreted.
		ItemID U3D //   40:64   Any client-defined UID or store any channel ID (TimeID,TimeID)
		EditID UID //   64:80   (edit.TimeID + replacing.TimeID) / 2
	}

	// AddressLSM is an Address serialized into its corresponding LSM key format.
	AddressLSM [AddressLength]byte

	// AddressID is AddressLSM sans an EditID.
	AddressID [AddressIDLength]byte

	// EditTable tracks the most recent EditID for a given AddressID.
	EditTable map[AddressID]UID

	// AddressRange expresses an ordered range of Addresses
	AddressRange struct {
		Lo     Address // inclusive lower bound
		Hi     Address // inclusive upper bound
		Weight float32 // > 0 includes, < 0 excludes, 0 no-op
	}

	OctalDigit    byte           // base 8 (3 least significant bits)
	OctalEncoding [64]OctalDigit // octal encoding of a U3D or UID
)

const (
	// Byte length of a serialized Address / AddressLSM.
	AddressLength = AddressIDLength + 16
	// Byte length of a serialized AddressID (an Address sans EditID).
	AddressIDLength = (8 + 16) + 16 + 24

	PackageTags  = "amp.spec.tag.v0.702"
	Base32Length = (8*24 + 4) / 5

	U3D_0_Max    = 0x7FFFFFFFFFFFFFFF // max allowed value of U3D[0]
	U3D_0_Symbol = 0xFFFFFFFFFFFFFFFF // signals a U3D symbolic token
	U3D_First    = 0x04               // symbol that matches first (smallest) U3D in a list
	U3D_Last     = 0x06               // symbol that matches last (largest) U3D in a list

	WithOperators   = `[\.+\s,\!\?]+`    // commutative (symmetric) binary delimiters
	ThenOperators   = `[\-/\\\~\:\^\@]+` // non-commutative binary or unary delimiter
	GroupDelimiters = `[]()<>{}¿?¡!`     // TODO group delimiter pairs

	// The "with" delimiter can be thought of as ADD or SUM and combines two terms in a commutative way like addition.
	// A '.' by convention helps visually identify an tag string, it's compatible with domain names, and is already a familiar scoping character.
	CanonicWith     = "."
	CanonicWithChar = byte('.')

	CanonicThen     = "-"
	CanonicThenChar = byte('-')

	GenesisEditClearBits = uint64(0xFFFFFF)
)
