package amp

import (
	"fmt"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var (
	// HeadCellID is a hardwired CellID and stores a tx's "head attributes"
	HeadCellID = tag.ID{0, 0, uint64(Const_HeadCellSeed)}
)

func MarshalPbToStore(src tag.ValuePb, dst []byte) ([]byte, error) {
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

func ErrorToValue(v error) tag.Value {
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

func (v *Tag) New() tag.Value {
	return &Tag{}
}

func (v *Tag) SetFromTime(t time.Time) {
	tag := tag.FromTime(t, false)
	v.ID_0 = int64(tag[0])
	v.ID_1 = tag[1]
	v.ID_2 = tag[2]
}

func (v *Tag) SetID(tagID tag.ID) {
	v.ID_0 = int64(tagID[0])
	v.ID_1 = tagID[1]
	v.ID_2 = tagID[2]
}

func (v *Tag) IsNil() bool {
	return v.URI != "" && v.Text == "" && v.ID_0 == 0 && v.ID_1 == 0 && v.ID_2 == 0
}

func (v *Tag) CompositeID() tag.ID {
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

func (v *Tag) MintNext(seed tag.ID, addEntropy bool) tag.ID {
	now := time.Now()
	timeID := tag.FromTime(now, addEntropy)
	mintID := seed.With(timeID).With(v.CompositeID())
	return mintID
}

func (v *Tag) AsLabel() string {
	if v.Text != "" {
		return v.Text
	}
	if v.ID_0 == 0 && v.ID_1 == 0 && v.ID_2 == 0 {
		return ""
	}
	tagID := tag.FromInts(int64(v.ID_0), v.ID_1, v.ID_2)
	return tagID.Base32()
}

func (v *Tags) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Tags) New() tag.Value {
	return &Tags{}
}

func (v *Tags) CompositeID() tag.ID {
	var composite tag.ID
	if v.Head != nil {
		composite = v.Head.CompositeID()
	}
	for i, subTag := range v.SubTags {
		subID := subTag.CompositeID()
		subID[2] ^= uint64(i+1) * uint64(Const_HeadCellSeed)
		composite = composite.With(subID)
	}
	return composite
}

func (v *Err) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Err) New() tag.Value {
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

func (v *Login) New() tag.Value {
	return &Login{}
}

func (v *LoginChallenge) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LoginChallenge) New() tag.Value {
	return &LoginChallenge{}
}

func (v *LoginResponse) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LoginResponse) New() tag.Value {
	return &LoginResponse{}
}

func (v *LoginCheckpoint) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LoginCheckpoint) New() tag.Value {
	return &LoginCheckpoint{}
}

func (v *PinRequest) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *PinRequest) New() tag.Value {
	return &PinRequest{}
}

func (v *PinRequest) PinTarget() tag.ID {
	if v.Select == nil {
		return tag.ID{}
	}
	return v.Select.CompositeID()
}

func (v *PinRequest) PinTag() *Tag {
	if v.Select == nil {
		return nil
	}
	return v.Select.Head
}

/*

// return standard go string getter
func (v *Request) String() string {
	tag := v.PinTag()
	return fmt.Sprintf("Request{%s}", v.Select.Tag.AsLiteral())
}

func (v *Request) Label() string {
	var strBuf [128]byte
	str := fmt.Appendf(strBuf[:0], "[req_%s] ", v.ID.Base32Suffix())
	if target := v.Select; target != nil {
		if target.URL != "" {
			str = fmt.Append(str, target.URL)
		}
	}
	return string(str)
}

*/
