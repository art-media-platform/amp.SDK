package data

// Value wraps a data element type, exposing tags, serialization, and instantiation methods.
type Value interface {
	ValuePb

	// Marshals this Value to a buffer, reallocating if needed.
	MarshalToStore(in []byte) (out []byte, err error)

	// Unmarshals and merges value state from a buffer.
	Unmarshal(src []byte) error

	// Creates a default instance of this same Tag type.
	New() Value
}

// Serialization shim for protobufs
type ValuePb interface {
	Size() int
	MarshalToSizedBuffer(dAtA []byte) (int, error)
	Unmarshal(dAtA []byte) error
}

// PbValue wraps a ValuePb (any protobuf-generated message) into a full data.Value.
// This avoids adding MarshalToStore() and New() boilerplate to every proto message type.
//
// Usage:  tx.Upsert(nodeID, attrID, itemID, amp.PbValue{epoch})
// type PbValue struct{ Val ValuePb }

// func (v PbValue) MarshalToStore(in []byte) (out []byte, err error) {
// 	return MarshalPbToStore(v.Val, in)
// }

// func (v PbValue) Unmarshal(src []byte) error {
// 	return v.Val.Unmarshal(src)
// }

// func (v PbValue) New() Value {
// 	return nil // PbValue is write-only; use the concrete type for deserialization
// }

// func (v PbValue) Size() int                                  { return v.Val.Size() }
// func (v PbValue) MarshalToSizedBuffer(b []byte) (int, error) { return v.Val.MarshalToSizedBuffer(b) }

func MarshalPbToStore(src ValuePb, dst []byte) ([]byte, error) {
	oldLen := len(dst)
	newLen := oldLen + src.Size()
	if cap(dst) < newLen {
		old := dst
		dst = make([]byte, (newLen+0x400)&^0x3FF)
		copy(dst, old)
	}
	dst = dst[:newLen]
	_, err := src.MarshalToSizedBuffer(dst[oldLen:])
	return dst, err
}
