package tag

import (
	"encoding/binary"
)

func (addr *Address) ElementID() (lsm ElementLSM) {
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
	binary.BigEndian.PutUint64(lsm[64:72], addr.FromID[0]) // FromID
	binary.BigEndian.PutUint64(lsm[72:80], addr.FromID[1]) //
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
	addr.FromID[0] = binary.BigEndian.Uint64(lsm[64:72]) // FromID
	addr.FromID[1] = binary.BigEndian.Uint64(lsm[72:80]) //
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

func (addr *ElementLSM) Set(nodeID, attrID, itemID UID) {
	binary.BigEndian.PutUint64((*addr)[0:8], nodeID[0])
	binary.BigEndian.PutUint64((*addr)[8:16], nodeID[1])
	binary.BigEndian.PutUint64((*addr)[16:24], attrID[0])
	binary.BigEndian.PutUint64((*addr)[24:32], attrID[1])
	binary.BigEndian.PutUint64((*addr)[32:40], itemID[0])
	binary.BigEndian.PutUint64((*addr)[40:48], itemID[1])
}

// Increments ItemID by 1, used to go to the next possible ElementLSM
func (addr *ElementLSM) NextItemID() bool {

	// From least to most significant byte of ItemID, add 1 until no carry
	for j := 1; j <= UID_Size; j++ {
		idx := ElementLSM_Size - j
		digit := addr[idx] + 1
		addr[idx] = digit
		if digit > 0 {
			return true // no carry, so we're done
		}
	}
	return false
}

func (addr AddressLSM) ElementLSM() ElementLSM {
	return ElementLSM(addr[0:ElementLSM_Size])
}
