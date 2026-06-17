// package amp provides core types and interfaces for art.media.platform.
package amp

import (
	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
	"google.golang.org/protobuf/proto"
)

// AppModuleInfo is the in-process handle an app hands to Registry.RegisterModule:
// what invokes the module (Tag, Aliases) plus the minimum needed to name and
// version it.  Its wire-facing projection (long-form About, glyphs, SBOM row) is
// std.ModuleRef, built via std.NewModuleRef.
type AppModuleInfo struct {
	Name    tag.Name // what invokes this module (registry lookup)
	About   string   // 1-line description -> ModuleRef.Labels.Caption
	Version string   // maturity "v{TRL}[.{minor}]"; see TRL-versioning.md
	Aliases []string // invocation aliases for an AppModule
}

// AppEnvironment is the runtime environment the amp runtime hands an AppContext: the app
// instance's identity within its home planet and its three file-system roots.
type AppEnvironment struct {
	HomeID      tag.UID // home node ID for this app instance
	MemberID    tag.UID // member identity running this app instance
	HomePath    string  // durable per-app read-write fs root (survives restarts)
	CachePath   string  // evictable per-app read-write scratch fs root
	BundledPath string  // read-only fs root of trusted factory/bundled assets
}

// AppModule is how an app module registers with amp.Host and is used for internal components as well as for third parties. During runtime, amp.Host instantiates an amp.AppModule when a client request invokes one of the app's registered tags.
//
// Like a traditional OS service, an amp.AppModule responds to queries it recognizes and operates on client requests. The stock amp runtime offers essential apps, such as file system access and user account services.
type AppModule struct {
	Info AppModuleInfo // identifying and invocation information

	// NewAppInstance is the instantiation entry point for an AppModule called when an AppModule is first invoked on a User session and is not yet running.
	//
	// Implementations should not block and should return quickly.
	NewAppInstance func(ctx AppContext) (AppInstance, error)
}

// AppContext is provided by the amp runtime to an AppInstance for support and context.
type AppContext interface {
	task.Context   // Allows select{} for graceful handling of app shutdown
	data.Publisher // Allows an app to publish assets for client consumption

	NewTx(scope ...TxScope) *TxMsg  // Creates a new tx, scoped to a target planet (default: home)
	Session() Session               // Access to underlying Session
	AppEnvironment() AppEnvironment // Runtime environment for this app instance
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

// Pinners process and "pin" requests, pushing responses to the client.
type Pinner interface {

	// Creates and serves the given request, providing a wrapper for the request.
	StartPin(req *Request) (Pin, error)
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

// TxMsg is the serialized transport container of CRDT (append-only modeled) operations
//
// A TxMsg carries one encryption context: TxEnvelope.Epoch selects a single planet or
// channel epoch key that encrypts the entire payload (TxHeader + TxOps + DataStore).
// All ops in a TxMsg must belong to the same encryption domain.  To write ops under
// different keys (e.g. two private channels), then author separate TxMsgs.
type TxMsg struct {
	TxEnvelope        // tx fields for tx routing and decryption (in the clear)
	TxHeader          // tx fields encrypted by Epoch key
	Ops        []TxOp // tx operations to perform
	DataStore  []byte // opaque data storage; typically serialized TxOp values
	Normalized bool   // normalization state of Ops
	refCount   int32  // see AddRef() / ReleaseRef()
	cryptOfs   uint64 // byte offset from preamble start to start of TxHeader
}

// TxOp is a transaction op and the most granular unit of change.
// A TxOp's serialized data is located in a TxMsg.DataStore or some other data segment.
type TxOp struct {
	Addr        tag.Address // CRDT item address
	Flags       TxOpFlags   // operation to perform
	AuthContext uint64      // AuthContext index that resides within TxHeader.AuthContexts
	DataOfs     uint64      // byte offset to where serialized data is stored
	DataLen     uint64      // byte length of associated serialized data
}

// TxScope is the optional NewTx parameter fixing a tx's target planet — the planet lever, set
// once at creation.  The zero value (and a bare NewTx()) targets the caller's home planet; set
// Planet to commit to an explicit planet.  Scope governs ROUTING only: signer identity
// (TxMsg.SetFromID) and privacy (TxMsg.Epoch) are independent levers set separately.  See
// AOM security-sync §7.6.
type TxScope struct {
	Planet tag.UID // unset → the caller's home planet; set → that explicit planet
}

// Binds an proto.Message prototype to its associated attribute tag.
type AttrDef struct {
	tag.Name                // maps the value Prototype to an explicit attr ID
	Prototype proto.Message // cloned when this attribute is instantiated
}
