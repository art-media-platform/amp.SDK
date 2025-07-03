package amp

import (
	"context"
	"net/url"

	"github.com/art-media-platform/amp.SDK/stdlib/media"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// Host allows app and transport services to be attached.
// Child processes attach as it responds to client requests to "pin" nodes via URLs.
type Host interface {
	task.Context

	// HostRegistry offers access to this Host's tag and amp.AppModule registry.
	HostRegistry() Registry

	// StartNewSession creates a new host session, binding the specified Transport to it.
	StartNewSession(parent HostService, via Transport) (Session, error)
}

// Transport wraps a TxMsg transport abstraction, allowing a Host to connect over any data transport layer.
// For example, tcp_service and lib_service each implement amp.Transport.
type Transport interface {

	// Describes this transport for logging and debugging.
	Label() string

	// Called when this Transport should close because the associated parent host session is closing or has closed.
	Close() error

	// SendTx sends a Msg to the remote client.
	// ErrNotConnected is used to denote normal stream close.
	SendTx(tx *TxMsg) error

	// RecvTx blocks until it receives a Msg or the stream is done.
	// ErrNotConnected is used to denote normal stream close.
	RecvTx() (*TxMsg, error)
}

// HostService attaches to a amp.Host as a child, extending host functionality.
type HostService interface {
	task.Context

	// StartService attaches a child task to a Host and starts this HostService.
	// This service may retain the amp.Host instance so that it can make calls to StartNewSession().
	StartService(on Host) error

	// StopService initiates a polite stop of this extension and blocks until it's in a "soft" closed state,
	//    meaning that its service has effectively stopped but its Context is still open.
	// Note this could take any amount of time (e.g. until all open requests are closed)
	// Typically, StopService() is called (blocking) and then Context.Close().
	// To stop immediately, Context.Close() is always available.
	StopService()
}

// TxCommit submits tx to be committed, a submission context, and where to send state.
type TxCommit struct {
	Tx      *TxMsg          // tx to commit
	Context context.Context // context for completion in that Done() aborts
	Origin  TxReceiver      // where to send replies and status updates
}

// TxReceiver handles / processes incoming tx
type TxReceiver interface {

	// Queues the given tx appropriately, aborting if ctx.Done() is signaled and returns ctx.Err()
	PushTx(tx *TxMsg, ctx context.Context) error
}

// Requester wraps a client request to receive one or more state updates.
type Requester interface {
	TxReceiver

	// Notifies this Requester of events during a Pin's life cycle.
	RecvEvent(evt PinEvent)
}

// Session in an open client session with an amp.Host.
// Closing is initiated via task.Context.Close().
type Session interface {
	task.Context // Underlying task context
	Registry     // How symbols and types registered and resolved
	TxReceiver   // Routes tx to a Session's Transport.RecvTx()

	// Returns the active media.Publisher instance for this session.
	AssetPublisher() media.Publisher

	// Returns info about this user and session -- READ ONLY
	Login() Login

	// Creates a new tx ready for use
	NewTx() *TxMsg

	// Submits a tx to this Session for processing, including who will receive replies and status updates.
	SubmitTx(commit TxCommit) error

	// Gets the requested currently running app instance.
	// If not running and autoCreate is set, a new instance is created and started.
	GetAppInstance(moduleID tag.UID, autoCreate bool) (AppInstance, error)
}

// Registry is where apps and types are registered -- concurrency safe.
type Registry interface {

	// Imports all the types and apps from another registry.
	// When a Session is created, its registry starts by importing the Host's registry.
	Import(other Registry) error

	// Registers a value as a prototype under its Attr.ID.
	// This allows the value to be instantiated and unmarshaled when an AttrID is known.
	RegisterAttr(def AttrDef) error

	// Registers an app by ID, URI, and schemas it supports.
	// Called by app modules (packages) at init() time.
	RegisterModule(app *AppModule) error

	// Selects an AppModule that best matches the given invocation.
	// Note that an *AppModule is READ ONLY since they are static and shared.
	GetAppModule(invoke Tag) (*AppModule, error)

	// Instantiates an attr element value for a given attr spec -- typically followed by Value.Unmarshal()
	MakeValue(attrID tag.UID) (Value, error)
}

// Parameter block for notifying a Requester
type PinEvent struct {
	Status PinStatus // pin status description
	Tx     *TxMsg    // relevant tx (if applicable)
	Error  error     // error if any for this event
}

// Request is a client request to pin a node or URL, offering many degrees of flexibility.
type Request struct {
	Requester             // origin of this request
	ItemFilter            // selects which nodes / attrs / items / edits to sync
	Tx         *TxMsg     // initial tx to process for this request
	ID         tag.UID    // universally unique ID for this request (inherited from tx invoking this request)
	InvokeURL  *url.URL   // initialized from PinRequest.Invoke.URI
	Params     url.Values // initialized from PinRequest.Invoke.URI
}

// ItemFilter is the accumulated state of all PinRequests made by the client.
type ItemFilter struct {
	Current  PinRequest   // current request state
	Selector ItemSelector // selects which items to emit / select
}

// CRDT kv entry pair
type ValueEntry struct {
	Addr  tag.Address // CRDT value element address
	Value Value       // initialized with default value of expected type
}

// Endpoint expresses a network protocol and address to bind / list / send to.
type Endpoint struct {
	Network string
	Address string
}
