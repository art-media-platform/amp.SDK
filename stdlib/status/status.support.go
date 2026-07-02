// Package status provides Status, an error type carried on the wire: a stable
// numeric Code (declared in status.proto; values are wire-frozen), an optional
// human-readable message, and a TimeID for ordering and deduplication.
// It also declares shared error values and error-wrapping helpers.
package status

import (
	"fmt"
	"slices"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
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
	ErrUnimplemented   = Code_Unimplemented.Error("not implemented")
	ErrInternal        = Code_AssertFailed.Error("internal error")
	ErrNotConnected    = Code_NotConnected.Error("not connected")
	ErrMalformedTx     = Code_MalformedTx.Error("malformed tx")
	ErrVerifyFailed    = Code_VerifySignatureFailed.Error("verification failed")
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

	// ErrEpochKeyNotFound means the epoch key needed for decryption or MemberProof
	// verification is not yet available in the Enclave.  Callers should retain the
	// TxMsg and retry when the key arrives (e.g. via a MemberEpoch distribution).
	ErrEpochKeyNotFound = Code_KeyringNotFound.Error("epoch key not found")
)

// Error implements the standard Go error interface, returning the message or the Code name when the message is empty.
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

func (v *Status) TimeID() tag.UID {
	return tag.UID{v.TimeID_0, v.TimeID_1}
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

// IsError reports whether err is a *Status carrying one of the given Codes.
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

func AsStatus(v error) *Status {
	if v == nil {
		return nil
	}
	status, _ := v.(*Status)
	if status == nil {
		wrapped := Code_Unnamed.Wrap(v)
		status = wrapped.(*Status)
	}
	return status
}

// Error returns a *Status with the given code and msg, stamped with a fresh TimeID.
// Returns nil if code == Code_Nil.
func (code Code) Error(msg string) error {
	if code == Code_Nil {
		return nil
	}
	tid := tag.NowID()
	return &Status{
		Code:     code,
		Message:  msg,
		TimeID_0: tid[0],
		TimeID_1: tid[1],
	}
}

// Errorf returns a *Status with the given code and msg.
// If one or more args are given, format is used as a format string.
func (code Code) Errorf(format string, msgArgs ...any) error {
	if code == Code_Nil {
		return nil
	}
	var msg string
	if len(msgArgs) == 0 {
		msg = format
	} else {
		msg = fmt.Sprintf(format, msgArgs...)
	}
	return code.Error(msg)
}

// Wrap returns a *Status with the given code and cause's message text; the cause chain is not retained.
// Returns nil if cause == nil.
func (code Code) Wrap(cause error) error {
	if cause == nil {
		return nil
	}
	return code.Error(cause.Error())
}

// Err returns a *Status with this code and no message; nil if code == Code_Nil.
func (code Code) Err() error {
	if code == Code_Nil {
		return nil
	}
	return code.Error("")
}
