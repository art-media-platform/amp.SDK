package tag

// ID is a signed 24 byte UTC time index in big endian form, with 6 bytes of signed seconds and 10 bytes of fractional precision.
// This means there are 47 bits dedicated to whole seconds, offering a range of ±4.4 million years.
//
// This also means (ID[0] >> 16) always yields a standard 64-bit Unix UTC timestamp.
type ID [3]uint64

// Specifies a set of tag literals and its corresponding tag.ID.
//
//	tag.Spec := "[{utf8_tag_literal}[TagDelimiters]*]*"
type Spec struct {
	ID      ID
	Canonic string
}

const (
	// TagDelimiters separate utf8 tag literals in tag strings.
	TagDelimiters = `[/\\\.+:\s\~]+`
)

// tag.Value wraps a data element type, exposing tags, serialization, and instantiation methods.
type Value interface {
	ValuePb

	// Returns the element type name (a scalar tag.Spec).
	TagSpec() Spec

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
