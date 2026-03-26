package status

import (
	"fmt"
	"slices"

	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/pkg/errors"
)

func Annotate(err *error, msg string, args ...any) {
	if *err != nil {
		*err = errors.Wrapf(*err, msg, args...)
	}
}

func AddStack(err *error) {
	if *err != nil {
		*err = errors.WithStack(*err)
	}
}

var (
	Err403             = errors.New("403 forbidden")
	Err404             = errors.New("not found")
	ErrConnection      = errors.New("connection failed")
	ErrClosed          = errors.New("closed")
	ErrUnsupported     = errors.New("unsupported")
	ErrUnimplemented   = Unimplemented.Error("not implemented")
	ErrInternal        = AssertFailed.Error("internal error")
	ErrNotConnected    = NotConnected.Error("not connected")
	ErrMalformedTx     = MalformedTx.Error("malformed tx")
	ErrBadTxEdit       = MalformedTx.Error("tx missing edit ID(s)")
	ErrBadTxOp         = MalformedTx.Error("bad value byte range")
	ErrInvalidLogin    = LoginFailed.Error("invalid login")
	ErrRequestExists   = BadRequest.Error("request already exists")
	ErrNoContext       = BadRequest.Error("context missing")
	ErrContextNotFound = ContextNotReady.Error("context not found")
	ErrValueDamaged    = StorageFailure.Error("value damaged")
	ErrNothingToCommit = CommitFailed.Error("nothing to commit")
	ErrAppNotFound     = ItemNotFound.Error("app module not found")
	ErrPathNotfound    = ItemNotFound.Error("path not found")
	ErrAttrNotFound    = ItemNotFound.Error("attribute not found")
	ErrItemNotFound    = ItemNotFound.Error("item not found")
	ErrRequestClosed   = RequestClosed.Error("request closed")
	ErrNotPinnable     = PinFailed.Error("not pinnable")
	ErrShuttingDown    = ShuttingDown.Error("shutting down")
	ErrTimeout         = Timeout.Error("timeout")
	ErrNoAuthToken     = AuthFailed.Error("no auth token")
	ErrCancelled       = Cancelled.Error("operation cancelled")
)

// Error makes our custom error type conform to a standard Go error
func (err *Error) Error() string {
	codeStr, exists := StatusCode_name[int32(err.Code)]
	if !exists {
		codeStr = StatusCode_name[int32(Unnamed)]
	}

	if len(err.Message) == 0 {
		return codeStr
	}
	return err.Message
}

func (v *Error) MarshalToStore(in []byte) (out []byte, err error) {
	return data.MarshalPbToStore(v, in)
}

func (v *Error) New() data.Value {
	return &Error{}
}

func GetCode(err error) StatusCode {
	if err == nil {
		return Nil
	}

	if artErr, ok := err.(*Error); ok {
		return artErr.Code
	}

	return Unnamed
}

// IsError tests if the given error is a Error error code (below).
// If err == nil, this returns false.
func IsError(err error, errCodes ...StatusCode) bool {
	if err == nil {
		return false
	}
	if perr, ok := err.(*Error); ok && perr != nil {
		if slices.Contains(errCodes, perr.Code) {
			return true
		}
	}

	return false
}

func ErrorToValue(v error) data.Value {
	if v == nil {
		return nil
	}
	err, _ := v.(*Error)
	if err == nil {
		wrapped := Unnamed.Wrap(v)
		err = wrapped.(*Error)
	}
	return err
}

// Error returns an *Error with the given error code
func (code StatusCode) Error(msg string) error {
	if code == Nil {
		return nil
	}
	return &Error{
		Code:    code,
		Message: msg,
	}
}

// Errorf returns an *Error with the given error code and msg.
// If one or more args are given, msg is used as a format string.
func (code StatusCode) Errorf(format string, msgArgs ...interface{}) error {
	if code == Nil {
		return nil
	}

	err := &Error{
		Code: code,
	}
	if len(msgArgs) == 0 {
		err.Message = format
	} else {
		err.Message = fmt.Sprintf(format, msgArgs...)
	}

	return err
}

// Wrap returns a ReqErr with the given error code and "cause" error
func (code StatusCode) Wrap(cause error) error {
	if cause == nil {
		return nil
	}
	return &Error{
		Code:    code,
		Message: cause.Error(),
	}
}

// Emits a generic error that wraps this status.Code
func (code StatusCode) Err() error {
	if code == Nil {
		return nil
	}
	return &Error{
		Code: code,
	}
}

// Formstatus.Error returns a status.Error with the given error code and msg set.
func (code StatusCode) FormError(msg string) error {
	if code == Nil {
		return nil
	}
	return &Error{
		Code:    code,
		Message: msg,
	}
}

// Formstatus.Errorf returns a status.Error with the given error code and formattable msg set.
func (code StatusCode) FormErrorf(msgFormat string, msgArgs ...interface{}) error {
	if code == Nil {
		return nil
	}
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(msgFormat, msgArgs...),
	}
}
