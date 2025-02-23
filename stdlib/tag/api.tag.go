package tag

// UTC16 is a 64 bit signed UTC timestamp with 16 bits of fractional precision.
//
// This format expresses timestamps within ±4.4 million years with 1/65536 second accuracy.
type UTC16 int64

// ID is a 24 byte LSM index, and leads with an 8 byte UTC16 timestamp.
//
// Since UTC16 is generally sufficient for applications, the trailing 16 bytes are available for any use,
// such as geo-tile IDs, lattice coordinates, or other LSM friendly values.
//
//		unixSecs := ID.Unix()   // ID[0] >> 16
//	    unixMs   := ID.UnixMs() // ID[0]*1000 >> 16
type ID [3]uint64

// Specifies a set of tag literals and its corresponding tag.ID.
//
//	tag.Expr := "[{TagOp}*[{utf8_token}]*"
type Expr struct {
	ID      ID     // hash of any art-media-platform or other tag expression
	Canonic string // optional UTF8 canonic tag expression that generates Expr.ID
}

const (
	PackageTags = "amp.spec.tag.v0.7000"

	WithDelimiters  = `[\.+\s,\:\!\?]+` // commutative (symmetric) binary delimiters
	ThenDelimiters  = `[\-/\\\~\^\@]+`  // non-commutative binary or unary delimiter
	GroupDelimiters = `[]()<>{}¿?¡!`    // TODO group delimiter pairs
)

// tag.Value wraps a data element type, exposing tags, serialization, and instantiation methods.
type Value interface {
	ValuePb

	// Marshals this Value to a buffer, reallocating if needed.
	MarshalToStore(in []byte) (out []byte, err error)

	// Unmarshals and merges value state from a buffer.
	Unmarshal(src []byte) error

	// Creates a default instance of this same Tag type
	New() Value
}

// ValuePb is a serialization shim for protobufs
type ValuePb interface {
	Size() int
	MarshalToSizedBuffer(dAtA []byte) (int, error)
	Unmarshal(dAtA []byte) error
}
