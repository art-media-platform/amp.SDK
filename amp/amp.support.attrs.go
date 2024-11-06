package amp

import (
	"fmt"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var (
	// CellID hard-wired to denote the root c
	MetaNodeID = tag.ID{0, 0, 2701}

	TagRoot  = tag.Spec{}.With("amp")
	AttrSpec = TagRoot.With("attr")
	AppSpec  = TagRoot.With("app")
)

func RegisterBuiltinTypes(reg Registry) error {

	prototypes := []tag.Value{
		&Err{},
		&Tag{},
		&LaunchURL{},
		&Login{},
		&LoginChallenge{},
		&LoginResponse{},
		&LoginCheckpoint{},
		&PinRequest{},
	}

	for _, pi := range prototypes {
		reg.RegisterPrototype(AttrSpec, pi, "")
	}

	return nil
}

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
	artErr, _ := v.(*Err)
	if artErr == nil {
		wrapped := ErrCode_Unnamed.Wrap(v)
		artErr = wrapped.(*Err)
	}
	return artErr
}

func (v *Tag) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Tag) TagSpec() tag.Spec {
	return AttrSpec.With("Tag")
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

func (v *Tags) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Tags) TagSpec() tag.Spec {
	return AttrSpec.With("Tags")
}

func (v *Tags) New() tag.Value {
	return &Tags{}
}

func (v *Tag) SetID(tagID tag.ID) {
	v.ID_0 = int64(tagID[0])
	v.ID_1 = tagID[1]
	v.ID_2 = tagID[2]
}

func (v *Tag) IsNil() bool {
	return v.URL != "" && v.UID == "" && v.Text == "" && v.ID_0 == 0 && v.ID_1 == 0 && v.ID_2 == 0
}

func (v *Tag) AsID() tag.ID {
	if v.ID_0 == 0 && v.ID_1 == 0 && v.ID_2 == 0 {
		if v.UID != "" {
			v.SetID(tag.FromLiteral([]byte(v.UID)))
		} else if v.Text != "" {
			v.SetID(tag.FromString(v.Text))
		}
	}
	return [3]uint64{
		uint64(v.ID_0),
		v.ID_1,
		v.ID_2,
	}
}

func (v *Tag) AsLiteral() string {
	if v.UID != "" {
		return v.UID
	}
	if v.Text != "" {
		return v.Text
	}
	if v.ID_0 == 0 && v.ID_1 == 0 && v.ID_2 == 0 {
		return ""
	}
	var tagID tag.ID
	tagID[0] = uint64(v.ID_0)
	tagID[1] = v.ID_1
	tagID[2] = v.ID_2
	return tagID.Base32()

}

func (v *Err) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Err) TagSpec() tag.Spec {
	return AttrSpec.With("Err")
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

func (v *LaunchURL) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LaunchURL) TagSpec() tag.Spec {
	return AttrSpec.With("LaunchURL")
}

func (v *LaunchURL) New() tag.Value {
	return &LaunchURL{}
}

func (v *Login) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *Login) TagSpec() tag.Spec {
	return AttrSpec.With("Login")
}

func (v *Login) New() tag.Value {
	return &Login{}
}

func (v *LoginChallenge) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LoginChallenge) TagSpec() tag.Spec {
	return AttrSpec.With("LoginChallenge")
}

func (v *LoginChallenge) New() tag.Value {
	return &LoginChallenge{}
}

func (v *LoginResponse) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LoginResponse) TagSpec() tag.Spec {
	return AttrSpec.With("LoginResponse")
}

func (v *LoginResponse) New() tag.Value {
	return &LoginResponse{}
}

func (v *LoginCheckpoint) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *LoginCheckpoint) TagSpec() tag.Spec {
	return AttrSpec.With("LoginCheckpoint")
}

func (v *LoginCheckpoint) New() tag.Value {
	return &LoginCheckpoint{}
}

func (v *PinRequest) MarshalToStore(in []byte) (out []byte, err error) {
	return MarshalPbToStore(v, in)
}

func (v *PinRequest) TagSpec() tag.Spec {
	return AttrSpec.With("PinRequest")
}

func (v *PinRequest) New() tag.Value {
	return &PinRequest{}
}

func (v *PinRequest) TargetID() tag.ID {
	target := v.PinTarget
	if target == nil {
		return tag.ID{}
	}
	return target.AsID()
}

/*
func (v *Request) AttrsToPin() map[tag.ID]struct{} {
	pinAttrs := make(map[tag.ID]struct{}, len(v.PinAttrs))
	for _, attr := range v.PinAttrs {
		attrID := attr.AttrID()
		if attrID.IsNil() && attr.URL != "" {
			attrID = tag.FormSpec(tag.Spec{}, attr.URL).ID
		}
		if !attrID.IsNil() {
			pinAttrs[attrID] = struct{}{}
		}
	}
	return pinAttrs
}
*/
