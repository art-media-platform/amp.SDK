package tag

import (
	"encoding/binary"
	"fmt"
)

func (addr *Address) ElementLSM() (lsm ElementLSM) {
	binary.BigEndian.PutUint64(lsm[0:8], addr.NodeID[0])   // NodeID
	binary.BigEndian.PutUint64(lsm[8:16], addr.NodeID[1])  //
	binary.BigEndian.PutUint64(lsm[16:24], addr.AttrID[0]) // AttrID
	binary.BigEndian.PutUint64(lsm[24:32], addr.AttrID[1]) //
	binary.BigEndian.PutUint64(lsm[32:40], addr.ItemID[0]) // ItemID
	binary.BigEndian.PutUint64(lsm[40:48], addr.ItemID[1]) //
	return
}

// Converts this Address to an AddressLSM.
func (addr *Address) AsLSM(lsm []byte) {
	binary.BigEndian.PutUint64(lsm[0:8], addr.NodeID[0])   // NodeID
	binary.BigEndian.PutUint64(lsm[8:16], addr.NodeID[1])  //
	binary.BigEndian.PutUint64(lsm[16:24], addr.AttrID[0]) // AttrID
	binary.BigEndian.PutUint64(lsm[24:32], addr.AttrID[1]) //
	binary.BigEndian.PutUint64(lsm[32:40], addr.ItemID[0]) // ItemID
	binary.BigEndian.PutUint64(lsm[40:48], addr.ItemID[1]) //
	binary.BigEndian.PutUint64(lsm[48:56], addr.EditID[0]) // EditID
	binary.BigEndian.PutUint64(lsm[56:64], addr.EditID[1]) //
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

// CompareElementID compares the NodeID, AttrID, and ItemID
func (addr *Address) CompareElementID(oth *Address) int {

	if addr.NodeID[0] < oth.NodeID[0] { // NodeID
		return -1
	}
	if addr.NodeID[0] > oth.NodeID[0] {
		return 1
	}
	if addr.NodeID[1] < oth.NodeID[1] {
		return -1
	}
	if addr.NodeID[1] > oth.NodeID[1] {
		return 1
	}

	if addr.AttrID[0] < oth.AttrID[0] { // AttrID
		return -1
	}
	if addr.AttrID[0] > oth.AttrID[0] {
		return 1
	}
	if addr.AttrID[1] < oth.AttrID[1] {
		return -1
	}
	if addr.AttrID[1] > oth.AttrID[1] {
		return 1
	}

	if addr.ItemID[0] < oth.ItemID[0] { // ItemID
		return -1
	}
	if addr.ItemID[0] > oth.ItemID[0] {
		return 1
	}
	if addr.ItemID[1] < oth.ItemID[1] {
		return -1
	}
	if addr.ItemID[1] > oth.ItemID[1] {
		return 1
	}

	return 0
}

func (addr *Address) Compare(oth *Address) int {
	diff := addr.CompareElementID(oth)
	if diff != 0 {
		return diff
	}

	if oth.EditID[0] < addr.EditID[0] { // REVERSED (newest first)
		return -1
	}
	if oth.EditID[0] > addr.EditID[0] { // REVERSED (newest first)
		return 1
	}
	if oth.EditID[1] < addr.EditID[1] { // REVERSED (newest first)
		return -1
	}
	if oth.EditID[1] > addr.EditID[1] { // REVERSED (newest first)
		return 1
	}

	return 0 // all equal
}

func (addr *Address) String() string {
	return fmt.Sprint(addr.ElementID.String(), ",", addr.EditID.String())
}

func (lsm *ElementLSM) ElementID() ElementID {
	var id ElementID
	id.NodeID[0] = binary.BigEndian.Uint64(lsm[0:8])
	id.NodeID[1] = binary.BigEndian.Uint64(lsm[8:16])
	id.AttrID[0] = binary.BigEndian.Uint64(lsm[16:24])
	id.AttrID[1] = binary.BigEndian.Uint64(lsm[24:32])
	id.ItemID[0] = binary.BigEndian.Uint64(lsm[32:40])
	id.ItemID[1] = binary.BigEndian.Uint64(lsm[40:48])
	return id
}

func (lsm *ElementLSM) Set(nodeID, attrID, itemID UID) {
	binary.BigEndian.PutUint64(lsm[0:8], nodeID[0])
	binary.BigEndian.PutUint64(lsm[8:16], nodeID[1])
	binary.BigEndian.PutUint64(lsm[16:24], attrID[0])
	binary.BigEndian.PutUint64(lsm[24:32], attrID[1])
	binary.BigEndian.PutUint64(lsm[32:40], itemID[0])
	binary.BigEndian.PutUint64(lsm[40:48], itemID[1])
}

func (lsm *ElementLSM) SetNodeID(nodeID UID) {
	binary.BigEndian.PutUint64(lsm[0:8], nodeID[0])
	binary.BigEndian.PutUint64(lsm[8:16], nodeID[1])
}

func (lsm *ElementLSM) NodeID() UID {
	return UID{
		binary.BigEndian.Uint64(lsm[0:8]),
		binary.BigEndian.Uint64(lsm[8:16]),
	}
}

func (lsm *ElementLSM) AttrID() UID {
	return UID{
		binary.BigEndian.Uint64(lsm[16:24]),
		binary.BigEndian.Uint64(lsm[24:32]),
	}
}

func (lsm *ElementLSM) ItemID() UID {
	return UID{
		binary.BigEndian.Uint64(lsm[32:40]),
		binary.BigEndian.Uint64(lsm[40:48]),
	}
}

func (lsm *ElementLSM) SetAttrID(attrID UID) {
	binary.BigEndian.PutUint64(lsm[16:24], attrID[0])
	binary.BigEndian.PutUint64(lsm[24:32], attrID[1])
}

func (lsm *ElementLSM) SetItemID(itemID UID) {
	binary.BigEndian.PutUint64(lsm[32:40], itemID[0])
	binary.BigEndian.PutUint64(lsm[40:48], itemID[1])
}

func (lsm *ElementLSM) DecrementNodeID() bool {
	return lsm.decrement(0)
}

func (lsm *ElementLSM) DecrementAttrID() bool {
	return lsm.decrement(UID_Size)
}

func (lsm *ElementLSM) DecrementItemID() bool {
	return lsm.decrement(2 * UID_Size)
}

func (lsm *ElementLSM) IncrementNodeID() bool {
	return lsm.increment(0)
}

func (lsm *ElementLSM) IncrementAttrID() bool {
	return lsm.increment(UID_Size)
}

func (lsm *ElementLSM) IncrementItemID() bool {
	return lsm.increment(2 * UID_Size)
}

// Decrements the implied UID, or returns false if the UID is already zero.
func (lsm *ElementLSM) decrement(offset int) bool {
	for j := UID_Size - 1; j >= 0; j-- {
		idx := offset + j
		if lsm[idx] > 0 {
			lsm[idx]--
			return true
		}
		lsm[idx] = 0xFF // borrow
	}
	return false // underflow
}

// Increments the implied UID, or returns false if ItemID is already at its maximum value.
func (lsm *ElementLSM) increment(offset int) bool {

	// From least to most significant byte of ItemID, add 1 until no carry
	for j := UID_Size - 1; j >= 0; j-- {
		idx := offset + j
		if lsm[idx] < 0xFF {
			lsm[idx]++
			return true
		}
		lsm[idx] = 0 // carry
	}
	return false // overflow
}

func (lsm *ElementLSM) String() string {
	elemID := lsm.ElementID()
	return elemID.String()
}

func (id *ElementID) String() string {
	return fmt.Sprint(id.NodeID.String(), ",", id.AttrID.String(), ",", id.ItemID.String())
}
