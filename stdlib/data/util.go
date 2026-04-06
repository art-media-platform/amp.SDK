package data

import (
	"google.golang.org/protobuf/proto"
)

var marshalOpts = proto.MarshalOptions{Deterministic: true}

// MarshalTo appends the deterministic wire encoding of msg to dst, reusing dst's capacity when possible.
func MarshalTo(dst []byte, msg proto.Message) ([]byte, error) {
	return marshalOpts.MarshalAppend(dst, msg)
}

// NewLike allocates a zero-value proto.Message of the same concrete type as msg.
func NewLike(msg proto.Message) proto.Message {
	return msg.ProtoReflect().New().Interface()
}

// Zeros out a given slice
func Zero(buf []byte) {
	N := int32(len(buf))
	for i := int32(0); i < N; i++ {
		buf[i] = 0
	}
}

// Encodes a int64 to a zig-zag uint64
func ToZigZag(x int64) uint64 {
	ux := uint64(x) << 1
	if x < 0 {
		ux = ^ux
	}
	return ux
}

// Decodes a zig-zag uint64 to a int64
func FromZigZag(ux uint64) int64 {
	x := ux >> 1
	if ux&1 != 0 {
		x = ^x
	}
	return int64(x)
}
