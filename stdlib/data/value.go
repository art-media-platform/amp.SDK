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
