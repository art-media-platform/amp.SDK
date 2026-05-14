package amp

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
	"google.golang.org/protobuf/proto"
)

// Host allows app and transport services to be attached.
// Child processes attach as it responds to client requests to "pin" nodes via URLs.
type Host interface {
	task.Context

	// HostRegistry offers access to this Host's tag and amp.AppModule registry.
	HostRegistry() Registry

	// StartNewSession creates a new host session, binding the specified Transport to it.
	StartNewSession(parent HostService, via Transport) (Session, error)

	// TxJournal returns the host's chronicle storage.  Wire-side endpoints
	// that surface raw chronicle replays (e.g. /api/v1/.../edits) read from
	// here; sealed planet-public TxMsgs decode via amp.OpenTx with nil crypto.
	TxJournal() TxJournal
}

// TransportInfo describes capabilities and requirements of a Transport.
type TransportInfo struct {

	// Describes this transport for logging and debugging.
	Label string

	// If set, the host must perform challenge-response authentication before granting access.
	// True for remote transports (e.g. TCP), false for local/embedded transports (e.g. lib).
	RequiresAuth bool
}

// Transport wraps a TxMsg transport abstraction, allowing a Host to connect over any data transport layer.
// For example, tcp_service and lib_service each implement amp.Transport.
type Transport interface {

	// Returns the parameters describing this transport's capabilities and requirements.
	Info() TransportInfo

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

	// Returns the active data.Publisher instance for this session.
	AssetPublisher() data.Publisher

	// Returns info about this user and session -- READ ONLY
	Login() *Login

	// Creates a new tx ready for use
	NewTx() *TxMsg

	// Submits a tx to this Session for processing, including who will receive replies and status updates.
	SubmitTx(commit TxCommit) error

	// Gets the requested currently running app instance.
	// If not running and autoCreate is set, a new instance is created and started.
	AppInstance(moduleID tag.UID, autoCreate bool) (AppInstance, error)

	// Returns the session's Enclave (identity key store), or nil if not yet initialized.
	Enclave() safe.Enclave

	// Sets the session's Enclave. Called by the home app after opening/creating it.
	SetEnclave(enc safe.Enclave)

	// Returns the session's EpochKeyStore (symmetric epoch keys), or nil if not yet initialized.
	EpochKeys() safe.EpochKeyStore

	// Sets the session's EpochKeyStore. Called by the home app after opening/creating it.
	SetEpochKeys(eks safe.EpochKeyStore)

	// Returns the current PlanetEpoch for a joined planet, or nil if not registered.
	Planet(planetID tag.UID) *PlanetEpoch

	// Registers or updates a planet's epoch in this session.
	// First call for a given planetID also joins the planet on the vault controller.
	//
	// Rotation-receipt atomicity contract — epoch installation MUST follow:
	//   (a) EpochKeyStore.PutKey for the new epoch's keys
	//   (b) Session.SetPlanet (this call)
	//   (c) Session.OnEpochKeyArrived
	// Any encrypted op dispatched after SetPlanet expects its key to already
	// be resolvable; inverting (a)/(b) is a latent race even on synchronous paths.
	SetPlanet(planetID tag.UID, epoch *PlanetEpoch)

	// Called after a new epoch key has been stored in EpochKeyStore.  Notifies
	// the vault controller to re-verify pending journal entries for this epoch.
	// See SetPlanet for the ordering contract this call closes.
	OnEpochKeyArrived(epochID tag.UID)

	// Processes a verified planet-public governance TxMsg (e.g. MemberEpoch distribution).
	// Called by the vault controller after signature verification succeeds.
	// Routes the TxMsg to all registered governance handlers for epoch key extraction.
	OnGovernanceTx(planetID tag.UID, tx *TxMsg)

	// Registers a handler to receive verified planet-public governance TxMsgs.
	// Apps call this during MakeReady to subscribe to governance events.
	RegisterGovernanceHandler(handler func(planetID tag.UID, tx *TxMsg))

	// StoreBlob hashes and stores blob data locally, returning a populated BlobRef.
	// The blob is stored encrypted in the host's BlobStore and queued for peer propagation
	// when the BlobRef is later committed in a TxMsg via SubmitTx.
	//
	// meta describes the blob's MIME type (ContentType), human label (Text), and
	// byte size (I with Units = Bytes, used as the progress denominator); or may be nil.
	// The stored BlobRef's BlobTag inherits ContentType and Text from meta; UID is set to
	// the leading 16 bytes of the plaintext hash (content-addressed, §13.2).
	//
	// For large files, data is streamed — not buffered in memory.
	// If onProgress is non-nil, it is called periodically with cumulative bytes written.
	StoreBlob(planetID tag.UID, src io.Reader, meta *Tag, onProgress func(bytesWritten int64)) (*BlobRef, error)

	// SeedBlob introduces a local file into a planet's blob pipeline. The host opens the
	// file directly (no IPC memcpy), hashes-and-stores in a single streaming pass, and
	// returns a populated BlobRef. Caller is expected to upsert the BlobRef into whatever
	// attr is appropriate on a target node (e.g. std.Attr.NodeBlobs keyed by BlobTag.UID).
	//
	// Content-addressed: re-seeding the same file produces the same BlobRef and is a no-op
	// at the BlobStore layer (§13.2). ContentType is inferred from the file extension.
	SeedBlob(planetID tag.UID, path string) (*BlobRef, error)

	// BlobStore returns the session's BlobStore for retrieving blobs by (planetID, blobID).
	// Apps use this to build data.Asset instances backed by stored blobs.
	BlobStore() BlobStore
}

// TxJournal stores raw TxMsg bytes keyed by (PlanetID, TxTimeID) for efficient range queries.
// This is the primary storage for the vault sync engine — it preserves the original wire-format
// TxMsg bytes for signature verification and peer-to-peer propagation.
//
// Quarantine: entries that fail cryptographic verification (bad MemberProof or bad signature)
// can be marked quarantined with a TTL.  Quarantined entries are suppressed from ReadSince
// (and therefore from fanout + sync propagation) while remaining retrievable via ReadQuarantined
// for audit.  HighWater and RangeHash continue to include quarantined entries so peers converge
// on the same journal shape.  The underlying store evicts quarantined entries after TTL.
type TxJournal interface {
	Close() error
	Append(planetID tag.UID, txTimeID tag.UID, raw []byte) error
	ReadSince(planetID tag.UID, after tag.UID, cb func(txTimeID tag.UID, raw []byte) bool) error
	HighWater(planetID tag.UID) (tag.UID, error)
	RangeHash(planetID tag.UID, start, end tag.UID) ([32]byte, error)

	// Quarantine rewrites the existing entry for (planetID, txTimeID) with a quarantine
	// flag and the given TTL.  Returns ErrNotFound-style status if the entry is absent.
	Quarantine(planetID tag.UID, txTimeID tag.UID, ttl time.Duration) error

	// ReadQuarantined iterates quarantined entries strictly after the `after` mark.
	// Parallel to ReadSince; returning false from the callback stops iteration.
	ReadQuarantined(planetID tag.UID, after tag.UID, cb func(txTimeID tag.UID, raw []byte) bool) error
}

// TxOutbox queues locally authored TxMsgs and blobs for propagation to vaults.
// Entries persist across restarts — the outbox is drained when vault connectivity is available.
type TxOutbox interface {
	EnqueueTx(planetID tag.UID, txTimeID tag.UID, raw []byte) error
	EnqueueBlob(ref *BlobRef) error
	DrainTx(cb func(planetID tag.UID, txTimeID tag.UID, raw []byte) error) error
	DrainBlobs(cb func(ref *BlobRef) error) error
	Close() error
}

// BlobStore manages content-addressed encrypted blob storage.
// Blobs are identified by (PlanetID, BlobID) and validated by their content hash.
type BlobStore interface {
	Store(planetID tag.UID, blobID tag.UID, data io.Reader, byteSize int64) error
	Retrieve(planetID tag.UID, blobID tag.UID) (io.ReadCloser, error)
	Has(planetID tag.UID, blobID tag.UID) bool

	// StoreHashed hashes and stores data content-addressed in a single streaming pass.
	// Caller pre-populates ref.PlanetID_0/1, ref.HashKitID (0 = default Blake2s_256), and
	// optionally ref.BlobTag.ContentType / ref.BlobTag.Text.
	// On success, StoreHashed populates ref.Hash_0..3 from the content hash, sets
	// ref.BlobTag.UID to the leading 16 bytes of that hash (§13.2), and sets
	// ref.BlobTag.I / ref.BlobTag.Units = Bytes to the authoritative plaintext byte count.
	// Idempotent: if the hash-derived on-disk path already exists, the temp write is discarded.
	StoreHashed(ref *BlobRef, data io.Reader, onProgress func(bytesWritten int64)) error
}

// Registry is where apps and types are registered -- concurrency safe.
type Registry interface {

	// Imports all the types and apps from another registry.
	// When a Session is created, its registry starts by importing the Host's registry.
	Import(other Registry) error

	// Registers a value as a prototype with a UID
	// This allows the value to be instantiated and unmarshaled when an AttrID is known.
	RegisterAttr(def AttrDef) error

	// Registers an app by ID, URI, and schemas it supports.
	// Called by app modules (packages) at init() time.
	RegisterModule(app *AppModule) error

	// Selects an AppModule that best matches the given invocation.
	// Note that an *AppModule is READ ONLY since they are static and shared.
	FindModule(uid tag.UID, name string) *AppModule

	// Instantiates a registered value having a given UID.
	NewValue(attrID tag.UID) (proto.Message, error)
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
	Tx         *TxMsg     // tx to process for this request
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
	Addr  tag.Address   // CRDT value element address
	Value proto.Message // initialized with default value of expected type
}

// Endpoint expresses a network protocol and address to bind / list / send to.
type Endpoint struct {
	Network string
	Address string
}
