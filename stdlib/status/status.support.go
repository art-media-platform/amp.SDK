package status

import (
	"fmt"
	"slices"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
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
	ErrUnimplemented   = Code_Unimplemented.Error("not implemented")
	ErrInternal        = Code_AssertFailed.Error("internal error")
	ErrNotConnected    = Code_NotConnected.Error("not connected")
	ErrMalformedTx     = Code_MalformedTx.Error("malformed tx")
	ErrBadTxEdit       = Code_MalformedTx.Error("tx missing edit ID(s)")
	ErrBadTxOp         = Code_MalformedTx.Error("bad value byte range")
	ErrInvalidLogin    = Code_LoginFailed.Error("invalid login")
	ErrRequestExists   = Code_BadRequest.Error("request already exists")
	ErrNoContext       = Code_BadRequest.Error("context missing")
	ErrContextNotFound = Code_ContextNotReady.Error("context not found")
	ErrValueDamaged    = Code_StorageFailure.Error("value damaged")
	ErrNothingToCommit = Code_CommitFailed.Error("nothing to commit")
	ErrAppNotFound     = Code_ItemNotFound.Error("app module not found")
	ErrPathNotfound    = Code_ItemNotFound.Error("path not found")
	ErrAttrNotFound    = Code_ItemNotFound.Error("attribute not found")
	ErrItemNotFound    = Code_ItemNotFound.Error("item not found")
	ErrRequestClosed   = Code_RequestClosed.Error("request closed")
	ErrNotPinnable     = Code_PinFailed.Error("not pinnable")
	ErrShuttingDown    = Code_ShuttingDown.Error("shutting down")
	ErrTimeout         = Code_Timeout.Error("timeout")
	ErrNoAuthToken     = Code_AuthFailed.Error("no auth token")
	ErrCancelled       = Code_Cancelled.Error("operation cancelled")
)

// Error makes our custom error type conform to a standard Go error
func (v *Status) Error() string {
	codeStr, exists := Code_name[int32(v.Code)]
	if !exists {
		codeStr = Code_name[int32(Code_Unnamed)]
	}

	if len(v.Message) == 0 {
		return codeStr
	}
	return v.Message
}

func GetCode(err error) Code {
	if err == nil {
		return Code_Nil
	}

	if entry, ok := err.(*Status); ok {
		return entry.Code
	}

	return Code_Unnamed
}

// IsError tests if the given error is a Error error code (below).
// If err == nil, this returns false.
func IsError(err error, errCodes ...Code) bool {
	if err == nil {
		return false
	}
	if perr, ok := err.(*Status); ok && perr != nil {
		if slices.Contains(errCodes, perr.Code) {
			return true
		}
	}

	return false
}

func ErrorToValue(v error) proto.Message {
	if v == nil {
		return nil
	}
	entry, _ := v.(*Status)
	if entry == nil {
		wrapped := Code_Unnamed.Wrap(v)
		entry = wrapped.(*Status)
	}
	return entry
}

// Error returns an *Error with the given error code
func (code Code) Error(msg string) error {
	if code == Code_Nil {
		return nil
	}
	return &Status{
		Code:    code,
		Message: msg,
	}
}

// Errorf returns an *Error with the given error code and msg.
// If one or more args are given, msg is used as a format string.
func (code Code) Errorf(format string, msgArgs ...interface{}) error {
	if code == Code_Nil {
		return nil
	}

	err := &Status{
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
func (code Code) Wrap(cause error) error {
	if cause == nil {
		return nil
	}
	return &Status{
		Code:    code,
		Message: cause.Error(),
	}
}

// Emits a generic error that wraps this std.Code_Code
func (code Code) Err() error {
	if code == Code_Nil {
		return nil
	}
	return &Status{
		Code: code,
	}
}

// Formstatus.Error returns a std.Code_Error with the given error code and msg set.
func (code Code) FormError(msg string) error {
	if code == Code_Nil {
		return nil
	}
	return &Status{
		Code:    code,
		Message: msg,
	}
}

// Formstatus.Errorf returns a std.Code_Error with the given error code and formattable msg set.
func (code Code) FormErrorf(msgFormat string, msgArgs ...interface{}) error {
	if code == Code_Nil {
		return nil
	}
	return &Status{
		Code:    code,
		Message: fmt.Sprintf(msgFormat, msgArgs...),
	}
}
