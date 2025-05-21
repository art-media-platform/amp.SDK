// package amp provides core types and interfaces for art.media.platform.
package amp

import (
	"github.com/art-media-platform/amp.SDK/stdlib/media"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

type AppModuleInfo struct {
	Tag          tag.Expr // what invokes this module
	Label        string   // human-readable description of this app
	Version      string   // "v{TRL}.{major}.{minor}"
	Dependencies tag.Expr // module Tags this app may access
	Aliases      []string // invocation aliases for an AppModule
}

type AppEnvironment struct {
	Session     Session       // session this app is running in
	Creator     AppModuleInfo // AppModule info that spawned this instance
	HomeID      tag.UID       // home ID for this app instance
	IID         tag.UID       // instance ID for this app instance spawned by AboutModule
	HomePath    string        // safe persistent read-write file system access
	CachePath   string        // safe persistent read-write file system access (low-priority)
	FactoryPath string        // safe read-only file system access "from factory"
}

// AppModule is how an app module registers with amp.Host and is used for internal components as well as for third parties. During runtime, amp.Host instantiates an amp.AppModule when a client request invokes one of the app's registered tags.
//
// Similar to a traditional OS service, an amp.AppModule responds to queries it recognizes and operates on client requests. The stock amp runtime offers essential apps, such as file system access and user account services.
type AppModule struct {
	Info AppModuleInfo // indentifying and invocation information

	// NewAppInstance is the instantiation entry point for an AppModule called when an AppModule is first invoked on a User session and is not yet running.
	//
	// Implementations should not block and should return quickly.
	NewAppInstance func(ctx AppContext) (AppInstance, error)
}

// AppContext is provided by the amp runtime to an AppInstance for support and context.
type AppContext interface {
	task.Context    // Allows select{} for graceful handling of app shutdown
	media.Publisher // Allows an app to publish assets for client consumption

	Session() Session               // Access to underlying Session
	AppEnvironment() AppEnvironment // Runtime environment for this app instance
}

// Pinners process and "pin" requests, pushing responses to the client.
type Pinner interface {

	// Creates and serves the given request, providing a wrapper for the request.
	StartPin(req *Request) (Pin, error)
}

// AppInstance is returned by an AppModule when it's invoked from a PinRequest.
type AppInstance interface {
	AppContext
	Pinner

	// Validates an incoming request and performs any needed setup before StartPin() is called.
	// This is a chance for an app to perform operations such as refreshing an auth token.
	MakeReady(req *Request) error

	// Called exactly once when this AppInstance has been closed.
	OnClosing()
}

// Pin is an attribute state connection to an app.
type Pin interface {
	Pinner

	// Revises this Pin's target sync state.
	ReviseRequest(latest *PinRequest) error

	// CommitTx calls tx.AddRef() and queues it for processing.
	// When complete, successful or not, Requester.RecvEvent() is called.
	CommitTx(tx *TxMsg) error

	// Context returns the task.Context associated with this Pin.
	// Apps start a Pin as a child Context of amp.AppContext.Context or as a child of another Pin.
	// This means an AppContext contains all its Pins, and Close() will close all Pins (and children).
	// This can be used to determine if a request is still being served and to close it if needed.
	Context() task.Context
}

// TxMsg is the serialized transport container sent between client and host.
type TxMsg struct {
	TxHeader         // public fields and routing tags
	Ops       []TxOp // tx operations to perform
	OpsSorted bool   // order state of []Ops
	DataStore []byte // TxOp serialzed data storage
	refCount  int32  // see AddRef() / ReleaseRef()
}

// TxOp is a transaction op and the most granular unit of change.
// A TxOp's serialized data is located in a TxMsg.DataStore or some other data segment.
type TxOp struct {
	Addr    tag.Address // element to operate on
	OpCode  TxOpCode    // operation to perform
	DataLen uint64      // byte length of associated serialized data
	DataOfs uint64      // byte offset to where serialized data is stored
}

// Binds an amp.Value prototype to its associated attribute tag.
type AttrDef struct {
	tag.Expr        // maps the value Prototype to an explicit attr ID
	Prototype Value // cloned when this attribute is instantiated
}

// Value wraps a data element type, exposing tags, serialization, and instantiation methods.
type Value interface {
	ValuePb

	// Marshals this Value to a buffer, reallocating if needed.
	MarshalToStore(in []byte) (out []byte, err error)

	// Unmarshals and merges value state from a buffer.
	Unmarshal(src []byte) error

	// Creates a default instance of this same Tag type.
	New() Value
}

// Serialization shim for protobufs
type ValuePb interface {
	Size() int
	MarshalToSizedBuffer(dAtA []byte) (int, error)
	Unmarshal(dAtA []byte) error
}
