package amp

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
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
	ampErr, _ := v.(*Error)
	if ampErr == nil {
		wrapped := ErrCode_Unnamed.Wrap(v)
		ampErr = wrapped.(*Error)
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

func (v *Error) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Error) New() Value {
	return &Error{}
}

// Emits a generic error that wraps this ErrCode
func (code ErrCode) Err() error {
	if code == ErrCode_Nil {
		return nil
	}
	return &Error{
		Code: code,
	}
}

// FormError returns a Error with the given error code and msg set.
func (code ErrCode) FormError(msg string) error {
	if code == ErrCode_Nil {
		return nil
	}
	return &Error{
		Code: code,
		Msg:  msg,
	}
}

// FormErrorf returns a Error with the given error code and formattable msg set.
func (code ErrCode) FormErrorf(msgFormat string, msgArgs ...interface{}) error {
	if code == ErrCode_Nil {
		return nil
	}
	return &Error{
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

func (v *PinRequest) AsLabel() string {
	if v == nil {
		return ""
	}

	label := make([]byte, 0, 255)
	if v.Invoke != nil {
		label = append(label, v.Invoke.AsLabel()...)
		return string(label)
	}

	if v.Selector != nil {
		label = append(label, ' ')
		label = append(label, v.Selector.AsLabel()...)
	}
	return string(label)
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

	if pinReq.Selector != nil {
		err := pinReq.Selector.Normalize(true)
		if err != nil {
			return err
		}
		request.ItemFilter.Selector = *pinReq.Selector
	}
	return nil
}

func (filter *ItemFilter) AsLabel() string {
	pinReq := &filter.Current

	label := make([]byte, 0, 255)
	if pinReq.Invoke != nil {
		label = append(label, pinReq.Invoke.AsLabel()...)
	}
	if pinReq.Selector != nil {
		label = append(label, '[')
		label = append(label, pinReq.Selector.AsLabel()...)
		label = append(label, ']')
	}
	return string(label)
}

// Returns if this range includes the given item's ElementID
func (filter *ItemFilter) Admits(elem tag.ElementID) bool {
	for _, span := range filter.Selector.Spans {
		nodeID := span.NodeID()
		if !nodeID.IsWildcard() && nodeID != elem.NodeID {
			continue
		}
		attrID := span.AttrID()
		if !attrID.IsWildcard() && attrID != elem.AttrID {
			continue
		}
		if elem.ItemID[0] < span.ItemID_Min_0 || elem.ItemID[0] > span.ItemID_Max_0 {
			continue
		}
		if elem.ItemID[1] < span.ItemID_Min_1 || elem.ItemID[1] > span.ItemID_Max_1 {
			continue
		}

		return true
	}
	return false
}

func (scan *NodeScan) NodeID() (nodeID tag.UID) {
	return tag.UID{
		scan.NodeID_0,
		scan.NodeID_1,
	}
}

func (scan *NodeScan) AttrID() (attrID tag.UID) {
	return tag.UID{
		scan.AttrID_0,
		scan.AttrID_1,
	}
}

func (scan *NodeScan) ItemRange() (min, max tag.UID) {
	min = tag.UID{
		scan.ItemID_Min_0,
		scan.ItemID_Min_1,
	}
	max = tag.UID{
		scan.ItemID_Max_0,
		scan.ItemID_Max_1,
	}
	return
}

func (scan *NodeScan) CompareTo(j *NodeScan) int {
	if scan.NodeID_0 < j.NodeID_0 { // NodeID
		return -1
	}
	if scan.NodeID_0 > j.NodeID_0 {
		return 1
	}
	if scan.NodeID_1 < j.NodeID_1 {
		return -1
	}
	if scan.NodeID_1 > j.NodeID_1 {
		return 1
	}

	if scan.AttrID_0 < j.AttrID_0 { // AttrID
		return -1
	}
	if scan.AttrID_0 > j.AttrID_0 {
		return 1
	}
	if scan.AttrID_1 < j.AttrID_1 {
		return -1
	}
	if scan.AttrID_1 > j.AttrID_1 {
		return 1
	}

	if scan.ItemID_Min_0 < j.ItemID_Min_0 { // ItemID Min
		return -1
	}
	if scan.ItemID_Min_0 > j.ItemID_Min_0 {
		return 1
	}
	if scan.ItemID_Min_1 < j.ItemID_Min_1 {
		return -1
	}
	if scan.ItemID_Min_1 > j.ItemID_Min_1 {
		return 1
	}

	if scan.ItemID_Max_0 < j.ItemID_Max_0 { // ItemID Max
		return -1
	}
	if scan.ItemID_Max_0 > j.ItemID_Max_0 {
		return 1
	}
	if scan.ItemID_Max_1 < j.ItemID_Max_1 {
		return -1
	}
	if scan.ItemID_Max_1 > j.ItemID_Max_1 {
		return 1
	}

	return 0 // equal
}

func (scan *NodeScan) AsLabel() string {
	if scan == nil {
		return ""
	}
	min, max := scan.ItemRange()
	return fmt.Sprintf("%s/%s/%s..%s", scan.NodeID().AsLabel(), scan.AttrID().AsLabel(), min.AsLabel(), max.AsLabel())
}

func (sel *ItemSelector) Select(elem tag.ElementID) {
	var itemMin, itemMax tag.UID
	if elem.ItemID.IsWildcard() {
		itemMax = tag.MaxID()
	} else {
		itemMin = elem.ItemID
		itemMax = elem.ItemID
	}

	sel.AddScan(elem.NodeID, elem.AttrID, itemMin, itemMax)
}

// Adds a selection range for all items on the given nodeID.
func (sel *ItemSelector) SelectNode(nodeID tag.UID) {
	sel.AddScan(nodeID, tag.WildcardID(), tag.UID{}, tag.MaxID())
}

// Adds a selection range for all items having the given nodeID and addrID.
func (sel *ItemSelector) SelectNodeAttr(nodeID, attrID tag.UID) {
	sel.AddScan(nodeID, attrID, tag.UID{}, tag.MaxID())
}

func (sel *ItemSelector) AddScan(nodeID, attrID, itemID_min, itemID_max tag.UID) {
	span := &NodeScan{
		NodeID_0: nodeID[0],
		NodeID_1: nodeID[1],

		AttrID_0: attrID[0],
		AttrID_1: attrID[1],

		ItemID_Min_0: itemID_min[0],
		ItemID_Min_1: itemID_min[1],
		ItemID_Max_0: itemID_max[0],
		ItemID_Max_1: itemID_max[1],

		EditsPerItem: 1,
	}

	sel.Scans = append(sel.Scans, span)
	sel.Normalized = false
}

func (sel *ItemSelector) Normalize(force bool) error {
	if !force && sel.Normalized {
		return nil
	}

	scans := sel.Scans
	N := len(scans)
	for i := 0; i < N; i++ {
		scan := scans[i]

		nodeID := scan.NodeID()
		if nodeID.IsNil() {
			return ErrCode_BadRequest.Error("NodeScan missing NodeID")
		}

		attrID := scan.AttrID()
		if attrID.IsNil() {
			return ErrCode_BadRequest.Error("NodeScan missing AttrID")
		}

		// enforce tag.UID_1_Max
		if scan.ItemID_Max_0 == tag.UID_0_Max && scan.ItemID_Max_1 > tag.UID_1_Max {
			scan.ItemID_Max_1 = tag.UID_1_Max
		}

		drop := false
		if scan.ItemID_Min_0 == tag.UID_0_Max && scan.ItemID_Min_1 > tag.UID_1_Max {
			drop = true
		} else if scan.EditsPerItem == 0 {
			drop = true
		} else if scan.ItemID_Min_0 > scan.ItemID_Max_0 || (scan.ItemID_Min_0 == scan.ItemID_Max_1 && scan.ItemID_Min_1 > scan.ItemID_Max_1) {
			drop = true
		}

		if drop {
			N--
			scans[i] = scans[N]
			i--
		}
	}

	// Reverse sort so that it plays nice with db reverse scan (to get newest EditID first per item)
	sort.Slice(scans, func(i, j int) bool {
		return scans[i].CompareTo(scans[j]) > 0 // REVERSE SORT
	})

	sel.Scans = scans[:N]
	sel.Normalized = true
	return nil
}

func (sel *ItemSelector) AsLabel() string {
	if sel == nil {
		return ""
	}
	N := len(sel.Scans)
	if N == 0 {
		return "{}"
	}
	parts := make([]string, 0, N)
	for _, scan := range sel.Scans {
		if scan == nil {
			continue
		}
		parts = append(parts, scan.AsLabel())
	}
	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))

}
