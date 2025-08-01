package amp

import (
	"fmt"
)

var (
	ErrUnimplemented   = ErrCode_Unimplemented.Error("not implemented")
	ErrInternal        = ErrCode_AssertFailed.Error("internal error")
	ErrNotConnected    = ErrCode_NotConnected.Error("not connected")
	ErrMalformedTx     = ErrCode_MalformedTx.Error("malformed tx")
	ErrBadTxEdit       = ErrCode_MalformedTx.Error("tx missing edit ID(s)")
	ErrBadTxOp         = ErrCode_MalformedTx.Error("bad value byte range")
	ErrInvalidLogin    = ErrCode_LoginFailed.Error("invalid login")
	ErrRequestExists   = ErrCode_BadRequest.Error("request already exists")
	ErrNoContext       = ErrCode_BadRequest.Error("context missing")
	ErrContextNotFound = ErrCode_ContextNotReady.Error("context not found")
	ErrValueDamaged    = ErrCode_StorageFailure.Error("value damaged")
	ErrNothingToCommit = ErrCode_CommitFailed.Error("nothing to commit")
	ErrAppNotFound     = ErrCode_ItemNotFound.Error("app module not found")
	ErrPathNotfound    = ErrCode_ItemNotFound.Error("path not found")
	ErrAttrNotFound    = ErrCode_ItemNotFound.Error("attribute not found")
	ErrItemNotFound    = ErrCode_ItemNotFound.Error("item not found")
	ErrRequestClosed   = ErrCode_RequestClosed.Error("request closed")
	ErrNotPinnable     = ErrCode_PinFailed.Error("not pinnable")
	ErrShuttingDown    = ErrCode_ShuttingDown.Error("shutting down")
	ErrTimeout         = ErrCode_Timeout.Error("timeout")
	ErrNoAuthToken     = ErrCode_AuthFailed.Error("no auth token")
	ErrCancelled       = ErrCode_Cancelled.Error("operation cancelled")
)

// Error makes our custom error type conform to a standard Go error
func (err *Error) Error() string {
	codeStr, exists := ErrCode_name[int32(err.Code)]
	if !exists {
		codeStr = ErrCode_name[int32(ErrCode_Unnamed)]
	}

	if len(err.Msg) == 0 {
		return codeStr
	}

	return err.Msg
}

// Error returns an *Error with the given error code
func (code ErrCode) Error(msg string) error {
	if code == ErrCode_Nil {
		return nil
	}
	return &Error{
		Code: code,
		Msg:  msg,
	}
}

// Errorf returns an *Error with the given error code and msg.
// If one or more args are given, msg is used as a format string.
func (code ErrCode) Errorf(format string, msgArgs ...interface{}) error {
	if code == ErrCode_Nil {
		return nil
	}

	err := &Error{
		Code: code,
	}
	if len(msgArgs) == 0 {
		err.Msg = format
	} else {
		err.Msg = fmt.Sprintf(format, msgArgs...)
	}

	return err
}

// IsError tests if the given error is a Error error code (below).
// If err == nil, this returns false.
func IsError(err error, errCodes ...ErrCode) bool {
	if err == nil {
		return false
	}
	if perr, ok := err.(*Error); ok && perr != nil {
		for _, errCode := range errCodes {
			if perr.Code == errCode {
				return true
			}
		}
	}

	return false
}

// Wrap returns a ReqErr with the given error code and "cause" error
func (code ErrCode) Wrap(cause error) error {
	if cause == nil {
		return nil
	}
	return &Error{
		Code: code,
		Msg:  cause.Error(),
	}
}

func GetErrCode(err error) ErrCode {
	if err == nil {
		return ErrCode_Nil
	}

	if artErr, ok := err.(*Error); ok {
		return artErr.Code
	}

	return ErrCode_Unnamed
}
