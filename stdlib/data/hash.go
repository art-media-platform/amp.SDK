package data

import "unsafe"

// HashBuf hashes data using the Go runtime's map hash (hardware-accelerated where available).
//
// WARNING: the hash seed is randomized per process, so values are NOT stable across
// runs or machines — never persist, wire-carry, or compare these hashes across processes.
func HashBuf(data []byte) uint64 {
	ss := (*stringStruct)(unsafe.Pointer(&data))
	return uint64(memhash(ss.str, 0, uintptr(ss.len)))
}

type stringStruct struct {
	str unsafe.Pointer
	len int
}

//go:noescape
//go:linkname memhash runtime.memhash
func memhash(p unsafe.Pointer, h, s uintptr) uintptr
