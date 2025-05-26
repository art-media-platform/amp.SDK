package amp

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var (
	// Bootstrapping aka "head" channel ID where to start.
	HeadChannelID = tag.UID{0, uint64(Const_HeadChannelID)}
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
			subID[2] ^= uint64(i+1) * uint64(Const_HeadChannelID)
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

	// Merge incoming PinRequest state changes
	// TODO: simple overwrite-style merge too basic?
	current := &request.PinFilter.Current
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

	err := request.PinFilter.Revise(pinReq)
	return err
}

func (filter *PinFilter) AsLabel() string {
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

// Exports this AddressRanges to a native and more accessible structure
func (sel *PinSelector) ExportRanges(dst *[]tag.AddressRange) {
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
		r.Lo.ChanID[0] = ri.Chan_Lo_0
		r.Lo.ChanID[1] = ri.Chan_Lo_1

		r.Lo.AttrID[0] = ri.Attr_Lo_0
		r.Lo.AttrID[1] = ri.Attr_Lo_1

		r.Lo.ItemID[0] = ri.Item_Lo_0
		r.Lo.ItemID[1] = ri.Item_Lo_1

		r.Lo.EditID[0] = ri.Edit_Lo_0
		r.Lo.EditID[1] = ri.Edit_Lo_1

		// Hi
		r.Hi.ChanID[0] = ri.Chan_Hi_0
		r.Hi.ChanID[1] = ri.Chan_Hi_1

		r.Hi.AttrID[0] = ri.Attr_Hi_0
		r.Hi.AttrID[1] = ri.Attr_Hi_1

		r.Hi.ItemID[0] = ri.Item_Hi_0
		r.Hi.ItemID[1] = ri.Item_Hi_1

		r.Hi.EditID[0] = ri.Edit_Hi_0
		r.Hi.EditID[1] = ri.Edit_Hi_1

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

func (filter *PinFilter) AddRange(rge tag.AddressRange) {
	filter.Select = append(filter.Select, rge)
}

func (filter *PinFilter) Revise(req *PinRequest) error {
	if filter == nil || req == nil {
		return nil
	}
	filter.Select = filter.Select[:0]
	req.Select.ExportRanges(&filter.Select)

	return nil
}

func (filter *PinFilter) NextRange(scan *tag.AddressRange) bool {

	// resume where the previous scan ended
	pos := scan.Hi
	for _, ri := range filter.Select {
		if ri.Hi.CompareTo(&pos, true) > 0 {
			scan.Lo = ri.Lo
			scan.Hi = ri.Hi
			return true
		}
	}
	return false
}

func (v *PinFilter) Admits(addr *tag.Address) bool {
	netWeight := float32(0)
	for _, ri := range v.Select {
		netWeight += ri.WeightAt(addr)
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

func (sel *PinSelector) AsLabel() string {
	if sel == nil {
		return ""
	}

	return "{TODO PinSelector AsLabel}"
	// label := make([]byte, 0, 255)
	// for _, r := range sel.Ranges {
	// 	if len(label) > 0 {
	// 		label = append(label, ',')
	// 	}
	// 	fmt.Appendf(label, run.AsLabel())
	// }
	// return string(label)
}

func (sel *PinSelector) AddSingle(addr tag.Address) {
	sel.AddRange(addr, addr)
}

func (sel *PinSelector) AddRange(lo, hi tag.Address) {
	if lo.CompareTo(&hi, true) > 0 {
		return
	}

	single := &AddrRange{
		Chan_Lo_0: lo.ChanID[0],
		Chan_Hi_0: hi.ChanID[0],
		Chan_Lo_1: lo.ChanID[1],
		Chan_Hi_1: hi.ChanID[1],

		Attr_Lo_0: lo.AttrID[0],
		Attr_Hi_0: hi.AttrID[0],
		Attr_Lo_1: lo.AttrID[1],
		Attr_Hi_1: hi.AttrID[1],

		Item_Lo_0: lo.ItemID[0],
		Item_Hi_0: hi.ItemID[0],
		Item_Lo_1: lo.ItemID[1],
		Item_Hi_1: hi.ItemID[1],

		Edit_Lo_0: lo.EditID[0],
		Edit_Lo_1: lo.EditID[1],
	}

	if hi.EditID.IsSet() {
		single.Edit_Hi_0 = hi.EditID[0]
		single.Edit_Hi_1 = hi.EditID[1]
	} else {
		single.Edit_Hi_0 = math.MaxUint64
		single.Edit_Hi_1 = math.MaxUint64
	}

	sel.Ranges = append(sel.Ranges, single)
}
