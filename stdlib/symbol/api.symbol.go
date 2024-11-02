package symbol

import (
	"encoding/binary"
	"errors"

	"github.com/art-media-platform/amp.SDK/stdlib/generics"
)

// ID is a persistent integer value associated with an immutable string or buffer value.
// ID == 0 always maps to the empty string / buf.
type ID uint32

// Ord returns the ordinal value of this ID (a type recasting to uint32)
func (id ID) Ord() uint32 {
	return uint32(id)
}

// IDSz is the byte size of a symbol.ID (big endian)
// The tradeoff is between key bytes idle (wasted) in a massive db and exponentially more IDs available.
//
// The thinking of a 4 byte ID is that an symbol table exceeding 100 million entries is impractical and inefficient.
// If a billion symbol IDs is "not enough"  then you are issuing IDs for the wrong purpose.
const IDSz = 4

// DefaultIssuerMin specifies the default minimum ID value for newly issued IDs.
//
// ID values less than this value are reserved for clients to represent hard-wired or "out of band" meaning.
// "Hard-wired" meaning that Table.SetSymbolID() can be called with IDs less than MinIssuedID without risk
// of an auto-issued ID contending with it.
const DefaultIssuerMin = 600

type Issuer interface {
	generics.RefCloser

	// Issues the next sequential unique ID, starting at MinIssuedID.
	IssueNextID() (ID, error)
}

var ErrIssuerNotOpen = errors.New("issuer not open")

// Table abstracts value-ID storage and two-way lookup.
type Table interface {
	generics.RefCloser

	// Returns the Issuer being used by this Table (passed via TableOpts.Issuer or auto-created if no TableOpts.Issuer was given)
	// Note that retained references should make use of generics.RefCloser to ensure proper closure.
	Issuer() Issuer

	// Returns the symbol ID associated with the given string/buffer value.
	// The given value buffer is never retained.
	//
	// If value not found and autoIssue == true, then a new ID is issued, bound to the given value, and true is returned.
	GetSymbolID(value []byte, autoIssue bool) (symbol ID, issued bool)

	// Associates the given buffer value to the given symbol ID, allowing multiple values to be mapped to a single ID.
	// If ID == 0, then this is the equivalent to GetSymbolID(value, true).
	SetSymbolID(value []byte, ID ID) (symbol ID, issued bool)

	// Looks up and appends the byte string associated with the given symbol ID to the given buf.
	// If ID is invalid or not found, nil is returned.
	GetSymbol(ID ID, io []byte) []byte
}

// Reads a big endian encoded uint32 ID from the given byte slice
func ReadID(in []byte) (uint32, []byte) {
	ID := binary.BigEndian.Uint32(in)
	return ID, in[IDSz:]
}

// Reads an ID from the given byte slice (reading IDSz=4 bytes)
func (id *ID) ReadFrom(in []byte) {
	*id = ID(binary.BigEndian.Uint32(in))
}

func AppendID(io []byte, ID uint32) []byte {
	return append(io, // big endian marshal
		byte(ID>>24),
		byte(ID>>16),
		byte(ID>>8),
		byte(ID))
}

func (id ID) AppendTo(io []byte) []byte {
	return append(io, // big endian marshal
		byte(uint32(id)>>24),
		byte(uint32(id)>>16),
		byte(uint32(id)>>8),
		byte(id))
}
