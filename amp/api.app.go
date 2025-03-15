// package amp provides core types and interfaces for art.media.platform.
package amp

import (
	"github.com/art-media-platform/amp.SDK/stdlib/media"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// App is how an app module registers with amp.Host and is used for internal components as well as for third parties. During runtime, amp.Host instantiates an amp.App when a client request invokes one of the app's registered tags.
//
// Similar to a traditional OS service, an amp.App responds to queries it recognizes and operates on client requests. The stock amp runtime offers essential apps, such as file system access and user account services.
type App struct {
	AppSpec      tag.Expr // unique and persistent ID for this module
	Desc         string   // human-readable description of this app
	Version      string   // "v{MajorVers}.{MinorID}.{RevID}"
	Dependencies []tag.ID // module Tags this app may access
	Invocations  []string // additional aliases that invoke this app

	// NewAppInstance is the instantiation entry point for an App called when an App is first invoked on a User session and is not yet running.
	//
	// Implementations should not block and return quickly.
	NewAppInstance func(ctx AppContext) (AppInstance, error)
}

// AppContext is provided by the amp runtime to an AppInstance for support and context.
type AppContext interface {
	task.Context      // Allows select{} for graceful handling of app shutdown
	media.Publisher   // Allows an app to publish assets for client consumption
	Session() Session // Access to underlying Session

	// Returns the absolute file system path of the app's local read-write directory.
	// This directory is scoped by App.AppSpec
	LocalDataPath() string

	// Gets the named attribute from the user's home storage -- used high-level app settings.
	// The attr is scoped by both the app Tag so key collision with other users or apps is not possible.
	// This is how an app can store and retrieve its settings for the current user.
	GetAppAttr(attrSpec tag.ID, dst tag.Value) error

	// Write analog for GetAppAttr()
	PutAppAttr(attrSpec tag.ID, src tag.Value) error
}

// Pinners process and "pin" request, pushing responses to the client.
type Pinner interface {

	// Creates and serves the given request, providing a wrapper for the request.
	ServeRequest(req Requester) (Pin, error)
}

// AppInstance is implemented by an App and invoked by amp.Host responding to a client pin request.
type AppInstance interface {
	AppContext
	Pinner

	// Validates a request and performs any needed setup.
	// This is a chance for an app to perform operations such refreshing an auth token.
	// Following this call, ServeRequest() is called.
	MakeReady(req Requester) error

	// Called exactly once when this App closes
	OnClosing()
}

// Pin is a attribute state connection to an amp.App.
// The handling App is responsible for updating the Requester with state changes as requested.
type Pin interface {
	Pinner

	// Context returns the task.Context associated with this Pin.
	// Apps start a Pin as a child Context of amp.AppContext.Context or as a child of another Pin.
	// This means an AppContext contains all its Pins, and Close() will close all Pins (and children).
	// This can be used to determine if a request is still being served and to close it if needed.
	Context() task.Context
}

// TxMsg is the the serialized transport container sent between client and host.
type TxMsg struct {
	TxHeader         // public fields and routing tags
	Ops       []TxOp // operations to perform on the target
	OpsSorted bool   // describes order of []Ops
	DataStore []byte // stores serialized TxOp data
	refCount  int32  // see AddRef() / ReleaseRef()
}

// ElementID is a multi-part LSM key consisting of CellID / AttrID / ItemID
type ElementID [3]tag.ID

// TxOp is a transaction op and the most granular unit of change.
// A TxOp's serialized data is located in a TxMsg.DataStore or some other data segment.
type TxOp struct {
	TxOpID           // applicable cell, attribute, element, and edit IDs
	OpCode  TxOpCode // operation to perform
	DataLen uint64   // byte length of associated serialized data
	DataOfs uint64   // byte offset to where serialized data is stored
}

// TxOpID is TxOp is a multi-part LSM key (CellID, AttrID, ItemID, EditID), forming an atomic CRDT entry ID.
// In other words, TxOpID is a LSM-based CRDT address scheme.
type TxOpID struct {
	CellID tag.ID // target cell / storage / container ID
	AttrID tag.ID // references an attribute or protocol specification
	ItemID tag.ID // user-defined UID, SKU, inline value, or element ID
	EditID tag.ID // references previous revision(s); see tag.ForkEdit()
}

// AttrDef specifies a value prototype and its associated attribute tag.
type AttrDef struct {
	tag.Expr            // maps the value Prototype to an explicit attr ID
	Prototype tag.Value // cloned when this attribute is instantiated
}
