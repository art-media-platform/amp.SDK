package tag

import (
	"bytes"
	"encoding/binary"
)

func (addr *Address) AsID() (lsm AddressID) {
	binary.BigEndian.PutUint64(lsm[0:8], addr.ChanID[0])   // ChanID
	binary.BigEndian.PutUint64(lsm[8:16], addr.ChanID[1])  //
	binary.BigEndian.PutUint64(lsm[16:24], addr.ChanID[2]) //
	binary.BigEndian.PutUint64(lsm[24:32], addr.AttrID[0]) // AttrID
	binary.BigEndian.PutUint64(lsm[32:40], addr.AttrID[1]) //
	binary.BigEndian.PutUint64(lsm[40:48], addr.ItemID[0]) // ItemID
	binary.BigEndian.PutUint64(lsm[48:56], addr.ItemID[1]) //
	binary.BigEndian.PutUint64(lsm[56:64], addr.ItemID[2]) //
	return
}

// Converts this Address to an AddressLSM.
func (addr *Address) AsLSM() (lsm AddressLSM) {
	binary.BigEndian.PutUint64(lsm[0:8], addr.ChanID[0])   // ChanID
	binary.BigEndian.PutUint64(lsm[8:16], addr.ChanID[1])  //
	binary.BigEndian.PutUint64(lsm[16:24], addr.ChanID[2]) //
	binary.BigEndian.PutUint64(lsm[24:32], addr.AttrID[0]) // AttrID
	binary.BigEndian.PutUint64(lsm[32:40], addr.AttrID[1]) //
	binary.BigEndian.PutUint64(lsm[40:48], addr.ItemID[0]) // ItemID
	binary.BigEndian.PutUint64(lsm[48:56], addr.ItemID[1]) //
	binary.BigEndian.PutUint64(lsm[56:64], addr.ItemID[2]) //
	binary.BigEndian.PutUint64(lsm[64:72], addr.EditID[0]) // EditID
	binary.BigEndian.PutUint64(lsm[72:80], addr.EditID[1]) //
	return
}

func (addr *Address) FromLSM(lsm []byte) {
	addr.ChanID[0] = binary.BigEndian.Uint64(lsm[0:8])   // ChanID
	addr.ChanID[1] = binary.BigEndian.Uint64(lsm[8:16])  //
	addr.ChanID[2] = binary.BigEndian.Uint64(lsm[16:24]) //
	addr.AttrID[0] = binary.BigEndian.Uint64(lsm[24:32]) // AttrID
	addr.AttrID[1] = binary.BigEndian.Uint64(lsm[32:40]) //
	addr.ItemID[0] = binary.BigEndian.Uint64(lsm[40:48]) // ItemID
	addr.ItemID[1] = binary.BigEndian.Uint64(lsm[48:56]) //
	addr.ItemID[2] = binary.BigEndian.Uint64(lsm[56:64]) //
	addr.EditID[0] = binary.BigEndian.Uint64(lsm[64:72]) // EditID
	addr.EditID[1] = binary.BigEndian.Uint64(lsm[72:80]) //
}

func (addr *Address) CompareTo(oth *Address, includeEditID bool) int {
	if addr.ChanID[0] < oth.ChanID[0] {
		return -1
	} else if addr.ChanID[0] > oth.ChanID[0] {
		return 1
	}
	if addr.ChanID[1] < oth.ChanID[1] {
		return -1
	} else if addr.ChanID[1] > oth.ChanID[1] {
		return 1
	}
	if addr.ChanID[2] < oth.ChanID[2] {
		return -1
	} else if addr.ChanID[2] > oth.ChanID[2] {
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
	if addr.ItemID[2] < oth.ItemID[2] {
		return -1
	} else if addr.ItemID[2] > oth.ItemID[2] {
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

// Returns "attraction" of the given Address in the range.
// weight: < 0 excludes, > 0 includes, 0 ignored
func (a *AddressRange) WeightAt(addr *Address) float32 {

	d_lo := a.Lo.CompareTo(addr, true)
	if d_lo > 0 {
		return 0
	}

	d_hi := a.Hi.CompareTo(addr, true)
	if d_hi < 0 {
		return 0
	}

	return a.Weight
}

func (a *AddressRange) CompareTo(b *AddressRange) int {
	dw := a.Weight - b.Weight
	if dw != 0 {
		return int(dw)
	}

	d := a.Lo.CompareTo(&b.Lo, true)
	if d != 0 {
		return d
	}

	d = a.Hi.CompareTo(&b.Hi, true)
	return d
}

func ChannelRange(chanID U3D) AddressRange {
	return AddressRange{
		Lo: Address{
			ChanID: chanID,
		},
		Hi: Address{
			ChanID: chanID,
			AttrID: UID_Max(),
			ItemID: U3D_Max(),
		},
	}
}

func AttrRange(chanID U3D, attrID UID) AddressRange {
	return AddressRange{
		Lo: Address{
			ChanID: chanID,
			AttrID: attrID,
		},
		Hi: Address{
			ChanID: chanID,
			AttrID: attrID,
			ItemID: U3D_Max(),
		},
	}
}
