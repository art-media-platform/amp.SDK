package amp

import (
	"encoding/binary"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var (
	// Bootstrapping aka "head" node ID where to start.
	HeadNodeID = tag.UID{0, uint64(Const_HeadNodeID)}
)

func TagFromUID(id tag.UID) *Tag {
	return &Tag{
		ID_0: id[0],
		ID_1: id[1],
	}
}

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

func ErrorToValue(v error) Value {
	if v == nil {
		return nil
	}
	ampErr, _ := v.(*Err)
	if ampErr == nil {
		wrapped := ErrCode_Unnamed.Wrap(v)
		ampErr = wrapped.(*Err)
	}
	return ampErr
}

func (v *Tag) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Tag) New() Value {
	return &Tag{}
}

func (v *Tag) SetFromTime(t time.Time) {
	id := tag.UID_FromTime(t)
	v.ID_0 = id[0]
	v.ID_1 = id[1]
}

func (v *Tag) SetID(uid tag.UID) {
	v.ID_0 = uid[0]
	v.ID_1 = uid[1]
}

func (v *Tag) IsNil() bool {
	return v != nil && v.URI == "" && v.ID_0 == 0 && v.ID_1 == 0
}

/*
	func (v *Tag) CompositeID() tag.UID {
		local := [3]uint64{
			uint64(v.R),
			uint64(v.I),
			uint64(v.J),
		}
		sum := tag.FromInts(int64(v.ID_0), v.ID_1, v.ID_2).With(local)
		if v.URI != "" {
			sum = sum.WithToken(v.URI)
		}
		if v.Text != "" {
			sum = sum.WithToken(v.Text)
		}
		return sum
	}

*/

func (v *Tag) UID() tag.UID {
	uid := tag.UID{}
	if v != nil {
		uid[0] = v.ID_0
		uid[1] = v.ID_1
	}
	return uid
}

func (v *Tag) AsLabel() string {
	str := make([]byte, 0, 128)

	if v.URI != "" {
		if len(str) > 0 {
			str = append(str, ',')
		}
		R := min(80, len(v.URI))
		str = append(str, v.URI[:R]...)
	}
	if v.Text != "" {
		if len(str) > 0 {
			str = append(str, '.')
		}
		R := min(80, len(v.Text))
		str = append(str, v.Text[:R]...)
	}
	id := v.UID()
	if id.IsSet() {
		if len(str) > 0 {
			str = append(str, '.')
		}
		str = append(str, id.AsLabel()...)
	}

	return string(str)
}

func (v *Tags) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Tags) New() Value {
	return &Tags{}
}

/*
	func (v *Tags) CompositeID() tag.UID {
		var composite tag.UID
		if v.Head != nil {
			composite = v.Head.CompositeID()
		}
		for i, subTag := range v.SubTags {
			subID := subTag.CompositeID()
			subID[2] ^= uint64(i+1) * uint64(Const_HeadNodeID)
			composite = composite.With(subID)
		}
		return composite
	}
*/
func (v *Err) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Err) New() Value {
	return &Err{}
}

// Err returns a Err with the given error code
func (code ErrCode) Err() error {
	if code == ErrCode_Nil {
		return nil
	}
	return &Err{
		Code: code,
	}
}

// FormError returns a Err with the given error code and msg set.
func (code ErrCode) FormError(msg string) error {
	if code == ErrCode_Nil {
		return nil
	}
	return &Err{
		Code: code,
		Msg:  msg,
	}
}

// FormErrorf returns a Err with the given error code and formattable msg set.
func (code ErrCode) FormErrorf(msgFormat string, msgArgs ...interface{}) error {
	if code == ErrCode_Nil {
		return nil
	}
	return &Err{
		Code: code,
		Msg:  fmt.Sprintf(msgFormat, msgArgs...),
	}
}

func (v *Login) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Login) New() Value {
	return &Login{}
}

func (v *LoginChallenge) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LoginChallenge) New() Value {
	return &LoginChallenge{}
}

func (v *LoginResponse) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LoginResponse) New() Value {
	return &LoginResponse{}
}

func (v *LoginCheckpoint) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LoginCheckpoint) New() Value {
	return &LoginCheckpoint{}
}

func (v *PinRequest) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *PinRequest) New() Value {
	return &PinRequest{}
}

func (req *Request) ParseParam(paramKey string, dst any) error {
	var paramStr string
	if paramStr = req.Params.Get(paramKey); paramStr == "" {
		return ErrCode_BadRequest.Errorf("missing param %q", paramKey)
	}

	switch v := dst.(type) {
	case *int:
		intVal, err := strconv.Atoi(paramStr)
		if err != nil {
			return ErrCode_BadRequest.Errorf("param %q is not an int: %v", paramKey, err)
		}
		*v = intVal
	case *string:
		*v = paramStr
	default:
		return ErrCode_BadRequest.Errorf("param %q is not a supported type: %T", paramKey, v)
	}

	return nil
}

func (request *Request) Revise(pinReq *PinRequest) error {
	if pinReq == nil {
		return nil
	}

	// Merge incoming PinRequest
	current := &request.ItemFilter.Current
	*current = *pinReq

	invokeTag := current.Invoke

	if invokeTag != nil && invokeTag.URI != "" {
		var err error
		if request.InvokeURL, err = url.Parse(invokeTag.URI); err != nil {
			err = ErrCode_BadRequest.Errorf("error parsing URL: %v", err)
			return err
		}
		if request.Params, err = url.ParseQuery(request.InvokeURL.RawQuery); err != nil {
			err = ErrCode_BadRequest.Errorf("error parsing URL query: %v", err)
			return err
		}
	}

	if pinReq.Select != nil {
		request.ItemFilter.Selector = *pinReq.Select
	}
	return nil
}

func (filter *ItemFilter) AsLabel() string {
	pinReq := &filter.Current

	label := make([]byte, 0, 255)
	if pinReq.Invoke != nil {
		label = append(label, pinReq.Invoke.AsLabel()...)
	}
	if pinReq.Select != nil {
		label = append(label, '[')
		label = append(label, pinReq.Select.AsLabel()...)
		label = append(label, ']')
	}
	return string(label)
}

/*
// Exports this AddressRanges to a native and more accessible structure
func (sel *ItemSelector) ExportRanges(dst *[]tag.AddressRange) {
	if sel == nil || len(sel.Ranges) == 0 {
		return
	}

	ranges := *dst

	for _, ri := range sel.Ranges {
		if ri == nil {
			continue
		}

		r := tag.AddressRange{
			Weight: 1,
		}

		// Lo
		r.Lo.NodeID[0] = ri.Node_Min_0
		r.Lo.NodeID[1] = ri.Node_Min_1

		r.Lo.AttrID[0] = ri.Attr_Min_0
		r.Lo.AttrID[1] = ri.Attr_Min_1

		r.Lo.ItemID[0] = ri.Item_Min_0
		r.Lo.ItemID[1] = ri.Item_Min_1

		r.Lo.EditID[0] = ri.Edit_Min_0
		r.Lo.EditID[1] = ri.Edit_Min_1

		// Hi
		r.Hi.NodeID[0] = ri.Node_Max_0
		r.Hi.NodeID[1] = ri.Node_Max_1

		r.Hi.AttrID[0] = ri.Attr_Max_0
		r.Hi.AttrID[1] = ri.Attr_Max_1

		r.Hi.ItemID[0] = ri.Item_Max_0
		r.Hi.ItemID[1] = ri.Item_Max_1

		r.Hi.EditID[0] = ri.Edit_Max_0
		r.Hi.EditID[1] = ri.Edit_Max_1

		ranges = append(ranges, r)
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].CompareTo(&ranges[j]) < 0
	})

	*dst = ranges

	{
		// TODO: consolidate overlapping or redundant ranges
	}
}

func (filter *ItemFilter) AddRange(rge tag.AddresRange) {
	filter.Select = append(filter.Select, rge)
}

func (filter *ItemFilter) Revise(req *PinRequest) error {
	if filter == nil || req == nil {
		return nil
	}
	filter.Selector.Ranges = append(filter.Selector.Ranges, req.Select.Ranges...)
	req.Select.ExportRanges(&filter.Select)

	return nil
}
*/

// Returns the next range to scan, starting from the given scan position.
// TODO: make iterator to iterate over ranges more efficiently.
func (filter *ItemFilter) NextRange(scan *AddrRange) bool {

	// resume where the previous scan ended
	pos := scan.Max()
	for _, ri := range filter.Selector.Ranges {
		ri_max := ri.Max()
		if ri_max.CompareTo(&pos) > 0 {
			scan.SetMin(ri.Min())
			scan.SetMax(ri_max)
			return true
		}
	}
	return false
}

func (r *AddrRange) Min() (lsm tag.AddressLSM) {
	binary.BigEndian.PutUint64(lsm[0:8], r.Node_Min_0)
	binary.BigEndian.PutUint64(lsm[8:16], r.Node_Min_1)
	binary.BigEndian.PutUint64(lsm[16:24], r.Attr_Min_0)
	binary.BigEndian.PutUint64(lsm[24:32], r.Attr_Min_1)
	binary.BigEndian.PutUint64(lsm[32:40], r.Item_Min_0)
	binary.BigEndian.PutUint64(lsm[40:48], r.Item_Min_1)
	binary.BigEndian.PutUint64(lsm[48:56], r.Edit_Min_0)
	return
}

func (r *AddrRange) Max() (lsm tag.AddressLSM) {
	binary.BigEndian.PutUint64(lsm[0:8], r.Node_Max_0)
	binary.BigEndian.PutUint64(lsm[8:16], r.Node_Max_1)
	binary.BigEndian.PutUint64(lsm[16:24], r.Attr_Max_0)
	binary.BigEndian.PutUint64(lsm[24:32], r.Attr_Max_1)
	binary.BigEndian.PutUint64(lsm[32:40], r.Item_Max_0)
	binary.BigEndian.PutUint64(lsm[40:48], r.Item_Max_1)
	binary.BigEndian.PutUint64(lsm[48:56], r.Edit_Max_0)
	return
}

func (r *AddrRange) SetMin(addr tag.AddressLSM) {
	r.Node_Min_0 = binary.BigEndian.Uint64(addr[0:8])
	r.Node_Min_1 = binary.BigEndian.Uint64(addr[8:16])
	r.Attr_Min_0 = binary.BigEndian.Uint64(addr[16:24])
	r.Attr_Min_1 = binary.BigEndian.Uint64(addr[24:32])
	r.Item_Min_0 = binary.BigEndian.Uint64(addr[32:40])
	r.Item_Min_1 = binary.BigEndian.Uint64(addr[40:48])
	r.Edit_Min_0 = binary.BigEndian.Uint64(addr[48:56])
}

func (r *AddrRange) SetMax(addr tag.AddressLSM) {
	r.Node_Max_0 = binary.BigEndian.Uint64(addr[0:8])
	r.Node_Max_1 = binary.BigEndian.Uint64(addr[8:16])
	r.Attr_Max_0 = binary.BigEndian.Uint64(addr[16:24])
	r.Attr_Max_1 = binary.BigEndian.Uint64(addr[24:32])
	r.Item_Max_0 = binary.BigEndian.Uint64(addr[32:40])
	r.Item_Max_1 = binary.BigEndian.Uint64(addr[40:48])
	r.Edit_Max_0 = binary.BigEndian.Uint64(addr[48:56])
}

// Returns selection weight of the given Address in the range:
// weight: < 0 excludes, > 0 includes, 0 ignored
func (rge *AddrRange) WeightAt(addr *tag.AddressLSM) float32 {

	min := rge.Min()
	cmp := min.CompareTo(addr)
	if cmp > 0 {
		return 0
	}

	max := rge.Max()
	cmp = max.CompareTo(addr)
	if cmp < 0 {
		return 0
	}

	return rge.Weight
}

func (v *ItemFilter) Admits(addr tag.AddressLSM) bool {
	netWeight := float32(0)
	for _, ri := range v.Selector.Ranges {
		netWeight += ri.WeightAt(&addr)
	}
	admits := netWeight > 0
	return admits
}

func (v *PinRequest) AsLabel() string {
	if v == nil {
		return ""
	}

	label := make([]byte, 0, 255)
	if v.Invoke != nil {
		label = append(label, v.Invoke.AsLabel()...)
		return string(label)
	}

	if v.Select != nil {
		label = append(label, ' ')
		label = append(label, v.Select.AsLabel()...)
	}
	return string(label)
}

func (sel *ItemSelector) AsLabel() string {
	if sel == nil {
		return ""
	}

	return "{TODO ItemSelector AsLabel}"
	// label := make([]byte, 0, 255)
	// for _, r := range sel.Ranges {
	// 	if len(label) > 0 {
	// 		label = append(label, ',')
	// 	}
	// 	fmt.Appendf(label, run.AsLabel())
	// }
	// return string(label)
}

// Adds a selection range for all items having the given nodeID and addrID.
func (sel *ItemSelector) AddNodeWithAddr(nodeID, addrID tag.UID) {
	min := tag.Address{
		NodeID: nodeID,
		AttrID: addrID,
	}
	max := tag.Address{
		NodeID: nodeID,
		AttrID: addrID,
		ItemID: tag.UID_Max(),
	}
	sel.AddRange(min, max, 1)
}

// Adds a selection range for all items on the given nodeID.
func (sel *ItemSelector) AddNode(nodeID tag.UID) {
	min := tag.Address{
		NodeID: nodeID,
	}
	max := tag.Address{
		NodeID: nodeID,
		AttrID: tag.UID_Max(),
		ItemID: tag.UID_Max(),
	}
	sel.AddRange(min, max, 1)
}

func (sel *ItemSelector) AddSingle(addr tag.Address) {
	sel.AddRange(addr, addr, 1)
}

func (sel *ItemSelector) AddRange(min, max tag.Address, weight float32) {
	if min.CompareTo(&max, true) > 0 {
		return
	}

	single := &AddrRange{
		Node_Min_0: min.NodeID[0],
		Node_Max_0: max.NodeID[0],
		Node_Min_1: min.NodeID[1],
		Node_Max_1: max.NodeID[1],

		Attr_Min_0: min.AttrID[0],
		Attr_Max_0: max.AttrID[0],
		Attr_Min_1: min.AttrID[1],
		Attr_Max_1: max.AttrID[1],

		Item_Min_0: min.ItemID[0],
		Item_Max_0: max.ItemID[0],
		Item_Min_1: min.ItemID[1],
		Item_Max_1: max.ItemID[1],

		Edit_Min_0: min.EditID[0],
		Edit_Min_1: min.EditID[1],
		Weight:     weight,
	}

	if max.EditID.IsSet() {
		single.Edit_Max_0 = max.EditID[0]
		single.Edit_Max_1 = max.EditID[1]
	} else {
		single.Edit_Max_0 = math.MaxUint64
		single.Edit_Max_1 = math.MaxUint64
	}

	sel.Ranges = append(sel.Ranges, single)
}
