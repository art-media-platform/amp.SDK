package amp

import (
	"net/url"

	"github.com/art-media-platform/amp.SDK/stdlib/media"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// Host allows app and transport services to be attached.
// Child processes attach as it responds to client requests to "pin" cells via URLs.
type Host interface {
	task.Context

	// Offers Go runtime and package level access to this Host's primary symbol and amp.App registry.
	// The amp.Registry interface bakes security and efficiently and tries to serve as effective package manager.
	HostRegistry() Registry

	// StartNewSession creates a new Session and binds its Msg transport to a stream.
	StartNewSession(parent HostService, via Transport) (Session, error)
}

// Transport wraps a Msg transport abstraction, allowing a Host to connect over any data transport layer.
// For example, a tcp-based transport as well as a dll-based transport are both implemented..
type Transport interface {

	// Describes this transport for logging and debugging.
	Label() string

	// Called when this stream should close because the associated parent host session is closing or has closed.
	Close() error

	// SendSTx sends a Msg to the remote client.
	// ErrStreamClosed is used to denote normal stream close.
	SendTx(tx *TxMsg) error

	// RecvTx blocks until it receives a Msg or the stream is done.
	// ErrStreamClosed is used to denote normal stream close.
	RecvTx() (*TxMsg, error)
}

// HostService attaches to a amp.Host as a child, extending host functionality.
type HostService interface {
	task.Context

	// StartService attaches a child task to a Host and starts this HostService.
	// This service may retain the amp.Host instance so that it can make calls to StartNewSession().
	StartService(on Host) error

	// GracefulStop initiates a polite stop of this extension and blocks until it's in a "soft" closed state,
	//    meaning that its service has effectively stopped but its Context is still open.
	// Note this could any amount of time (e.g. until all open requests are closed)
	// Typically, GracefulStop() is called (blocking) and then Context.Close().
	// To stop immediately, Context.Close() is always available.
	GracefulStop()
}

// Session in an open client session with an amp.Host.
// Closing is initiated via task.Context.Close().
type Session interface {
	task.Context // Underlying task context
	Registry     // How symbols and types registered and resolved

	// Returns the active media.Publisher instance for this session.
	AssetPublisher() media.Publisher

	// Returns info about this user and session
	Login() Login

	// Sends a readied Msg to the client for handling.
	// On exit, the given msg should not be referenced further.
	SendTx(tx *TxMsg) error

	// Gets the currently running AppInstance for an AppID.
	// If the requested app is not running and autoCreate is set, a new instance is created and started.
	GetAppInstance(appID tag.ID, autoCreate bool) (AppInstance, error)
}

// Registry is where apps and types are registered -- concurrency safe.
type Registry interface {

	// Imports all the types and apps from another registry.
	// When a Session is created, its registry starts by importing the Host's registry.
	Import(other Registry) error

	// Registers a value as a prototype under its Attr.ID.
	// This allows the value to be instantiated and unmarshaled when an AttrID is known.
	RegisterAttr(def AttrDef) error

	// Registers an app by its UTag, URI, and schemas it supports.
	RegisterApp(app *App) error

	// Looks-up an app by tag ID -- READ ONLY ACCESS
	GetAppByTag(appTag tag.ID) (*App, error)

	// Selects the app that best matches an invocation string.
	GetAppForInvocation(invocation string) (*App, error)

	// Instantiates an attr element value for a given attr spec -- typically followed by tag.Value.Unmarshal()
	MakeValue(attrSpec tag.ID) (tag.Value, error)
}

// Requester wraps a client request to receive one or more state updates.
type Requester interface {

	// Returns all Request parameters.
	Request() *Request

	// Submits tx to this Requester for processing.
	PushTx(tx *TxMsg) error

	// Called by a Pin to notify its Requester that service is complete (successfully or not)
	// No-op if the Requester is was already complete or was cancelled.
	OnComplete(err error)
}

// Request is a client request to pin a cell or URL, offering many degrees of flexibility.
type Request struct {
	PinRequest            // Raw client request
	ID         tag.ID     // Universally unique genesis ID for this request
	CommitTx   *TxMsg     // if non-nil, this tx is committed to be merged
	URL        *url.URL   // Initialized from PinRequest.PinTarget.URL (or nil if missing)
	Values     url.Values // Initialized from PinRequest.PinTarget.URL (or nil if missing)
}
