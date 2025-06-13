package tag

import (
	"bytes"
	"encoding/binary"
)

func (addr *Address) ElemID() (lsm AddressID) {
	binary.BigEndian.PutUint64(lsm[0:8], addr.NodeID[0])   // NodeID
	binary.BigEndian.PutUint64(lsm[8:16], addr.NodeID[1])  //
	binary.BigEndian.PutUint64(lsm[16:24], addr.AttrID[0]) // AttrID
	binary.BigEndian.PutUint64(lsm[24:32], addr.AttrID[1]) //
	binary.BigEndian.PutUint64(lsm[32:40], addr.ItemID[0]) // ItemID
	binary.BigEndian.PutUint64(lsm[40:48], addr.ItemID[1]) //
	return
}

// Converts this Address to an AddressLSM.
func (addr *Address) AsLSM() (lsm AddressLSM) {
	binary.BigEndian.PutUint64(lsm[0:8], addr.NodeID[0])   // NodeID
	binary.BigEndian.PutUint64(lsm[8:16], addr.NodeID[1])  //
	binary.BigEndian.PutUint64(lsm[16:24], addr.AttrID[0]) // AttrID
	binary.BigEndian.PutUint64(lsm[24:32], addr.AttrID[1]) //
	binary.BigEndian.PutUint64(lsm[32:40], addr.ItemID[0]) // ItemID
	binary.BigEndian.PutUint64(lsm[40:48], addr.ItemID[1]) //
	binary.BigEndian.PutUint64(lsm[48:56], addr.EditID[0]) // EditID
	binary.BigEndian.PutUint64(lsm[56:64], addr.EditID[1]) //
	return
}

func (addr *Address) FromLSM(lsm []byte) {
	addr.NodeID[0] = binary.BigEndian.Uint64(lsm[0:8])   // NodeID
	addr.NodeID[1] = binary.BigEndian.Uint64(lsm[8:16])  //
	addr.AttrID[0] = binary.BigEndian.Uint64(lsm[16:24]) // AttrID
	addr.AttrID[1] = binary.BigEndian.Uint64(lsm[24:32]) //
	addr.ItemID[0] = binary.BigEndian.Uint64(lsm[32:40]) // ItemID
	addr.ItemID[1] = binary.BigEndian.Uint64(lsm[40:48]) //
	addr.EditID[0] = binary.BigEndian.Uint64(lsm[48:56]) // EditID
	addr.EditID[1] = binary.BigEndian.Uint64(lsm[56:64]) //
}

func (addr *Address) CompareTo(oth *Address, includeEditID bool) int {
	if addr.NodeID[0] < oth.NodeID[0] {
		return -1
	} else if addr.NodeID[0] > oth.NodeID[0] {
		return 1
	}
	if addr.NodeID[1] < oth.NodeID[1] {
		return -1
	} else if addr.NodeID[1] > oth.NodeID[1] {
		return 1
	}

	if addr.AttrID[0] < oth.AttrID[0] {
		return -1
	} else if addr.AttrID[0] > oth.AttrID[0] {
		return 1
	}
	if addr.AttrID[1] < oth.AttrID[1] {
		return -1
	} else if addr.AttrID[1] > oth.AttrID[1] {
		return 1
	}

	if addr.ItemID[0] < oth.ItemID[0] {
		return -1
	} else if addr.ItemID[0] > oth.ItemID[0] {
		return 1
	}
	if addr.ItemID[1] < oth.ItemID[1] {
		return -1
	} else if addr.ItemID[1] > oth.ItemID[1] {
		return 1
	}

	if !includeEditID {
		return 0 // equal sans EditID
	}
	if addr.EditID[0] < oth.EditID[0] {
		return -1
	} else if addr.EditID[0] > oth.EditID[0] {
		return 1
	}
	if addr.EditID[1] < oth.EditID[1] {
		return -1
	} else if addr.EditID[1] > oth.EditID[1] {
		return 1
	}
	return 0 // all equal
}

const (
	kItemOfs = AddressIDLength - 24
	kEditOfs = AddressIDLength
)

// Increments Address.ItemID by 1 and zeros out the EditID
func (addr *AddressLSM) NextItemID() {

	for i := kEditOfs - 1; i >= kItemOfs; i-- {
		digit := addr[i] + 1
		addr[i] = digit
		if digit > 0 {
			break // no carry means add 1 complete
		}
	}

	// Zero out the EditID
	for i := kEditOfs; i < AddressLength; i++ {
		addr[i] = 0
	}
}

func (addr *AddressLSM) AsID() AddressID {
	return AddressID(addr[0:AddressIDLength])
}

func (addr *AddressLSM) CompareTo(oth *AddressLSM) int {
	return bytes.Compare(addr[:], oth[:])
}
