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

	// Invoke marks an in-process verb-RPC: the host delivers Tx's ops to the app
	// verb named by Tx.Request.URL as StartPin arguments WITHOUT sealing or
	// journaling them as planet state — the request is not signed, not written to
	// the journal/outbox, and never propagated to peers, so it must originate from
	// an already-authenticated local session.  The verb authors any durable writes
	// itself (e.g. a custodial Commit).  This is a host-internal submit flag, not a
	// wire field: verb-RPC is local by construction and carries no wire mode.
	Invoke bool
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

	// Creates a new tx ready for use, scoped to a target planet (default: home).
	NewTx(scope ...TxScope) *TxMsg

	// Submits a tx to this Session for processing, including who will receive replies and status updates.
	SubmitTx(commit TxCommit) error

	// Gets the requested currently running app instance.
	// If not running and autoCreate is set, a new instance is created and started.
	AppInstance(moduleID tag.UID, autoCreate bool) (AppInstance, error)

	// Returns the current PlanetEpoch for a joined planet, or nil if not registered.
	Planet(planetID tag.UID) *PlanetEpoch

	// DialVaultPeers asks the vault controller to dial peer addresses learned at
	// runtime — the VaultAddrs carried by a PlanetInvite or a NameService record —
	// so a fresh peer can bootstrap a connection without a static, operator-
	// configured peer.  Best-effort and asynchronous; a no-op when the host runs
	// without a vault transport.  See vault.Transport.AddPeer.
	DialVaultPeers(addrs []*VaultAddr) error

	// WatchPlanet starts syncing a planet's journal without joining as a member —
	// the "pin" half of resolve→pin: a consumer that resolved a name (or holds a
	// planet UID) watches it so its planet-public records stream in.  Distinct from
	// HostSession.SetPlanet, which joins with an epoch + keys.  No-op without a vault transport.
	WatchPlanet(planetID tag.UID) error

	// Privileged returns this session's host-privileged capabilities, or nil when
	// the session is not the in-process host session (test doubles, and future
	// out-of-process app sandboxes, return nil).  First-party governance apps
	// require it and fail closed when absent.
	Privileged() HostSession

	// PlanetMember returns the member identity this session has adopted on
	// planetID.  For planets the session founded or owns, this is the login
	// member; for planets joined via invite it is the freshly generated identity
	// the session introduced on accept.  A session holds several adopted
	// identities — one per planet it is invited to — so the signer for a tx is
	// the identity adopted on that tx's planet, never a single mutable identity.
	// Falls back to the login member when no per-planet identity is recorded.
	PlanetMember(planetID tag.UID) tag.UID

	// Registers a handler to receive verified planet-public governance TxMsgs.
	// Apps call this during MakeReady to subscribe to governance events.
	RegisterGovernanceHandler(handler func(planetID tag.UID, tx *TxMsg))

	// StoreBlob hashes and stores blob data locally, returning a populated BlobRef.
	// The blob is stored encrypted in the host's BlobStore and queued for peer propagation
	// when the BlobRef is later committed in a TxMsg via SubmitTx.
	//
	// meta describes the blob's MIME type (ContentType), human label (Text), and
	// byte size (I with Units = Bytes, used as the progress denominator); or may be nil.
	// The stored BlobRef's AssetTag carries ContentType and Text from meta; AssetTag.UID is the
	// leading 16 bytes of the plaintext hash (content-addressed, §13.2), and for a planet-public
	// blob BlobTag.UID coincides with it.  BlobTag itself stays lean — UID + stored byte count.
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

	// OpenBlob resolves a BlobRef to a seekable plaintext reader — the read-side twin of StoreBlob.
	// An epoch-sealed blob is retrieved as ciphertext, decrypted once under its epoch key, and the
	// recovered plaintext is validated against the asset hash (Hash_0..3, §13.5); a public blob
	// streams straight from the BlobStore. Decrypted plaintext is served from a Tier-2 cache so
	// repeat reads (e.g. HTTP range requests while scrubbing media) skip the decrypt. Apps use this
	// to back a data.Asset over a stored blob.
	OpenBlob(planetID tag.UID, ref *BlobRef) (data.AssetReader, error)

	// PrefetchBlobs names refs as admitted for the planet and pulls any still missing —
	// the admission surface for a playback-queue or spatial-neighborhood prefetch signal
	// (SD-planet-storage §13.5, D17).  Best-effort and asynchronous; a no-op without vault sync.
	PrefetchBlobs(planetID tag.UID, refs []*BlobRef)
}

// HostSession is the host's privileged session surface, deliberately OFF the
// public read/write Session API — one authoritative interface replacing the
// per-concern downcast seams.  The host's session is the sole implementation.
type HostSession interface {

	// key custody

	// Returns the session's Enclave (identity key store), or nil if not yet initialized.
	Enclave() safe.Enclave

	// Sets the session's Enclave. Called by the home app after opening/creating it.
	SetEnclave(enc safe.Enclave)

	// Returns the session's EpochKeyStore (symmetric epoch keys), or nil if not yet initialized.
	EpochKeys() safe.EpochKeyStore

	// Sets the session's EpochKeyStore. Called by the home app after opening/creating it.
	SetEpochKeys(eks safe.EpochKeyStore)

	// planet / epoch control

	// Registers or updates a planet's epoch in this session.
	// First call for a given planetID also joins the planet on the vault controller.
	//
	// Rotation-receipt atomicity contract — epoch installation MUST follow:
	//   (a) EpochKeyStore.PutKey for the new epoch's keys
	//   (b) HostSession.SetPlanet (this call)
	//   (c) HostSession.OnEpochKeyArrived
	// Any encrypted op dispatched after SetPlanet expects its key to already
	// be resolvable; inverting (a)/(b) is a latent race even on synchronous paths.
	SetPlanet(planetID tag.UID, epoch *PlanetEpoch)

	// SetPlanetMember records the member identity adopted on planetID.  Called by
	// the home app on InviteAccept so later txs on that planet are attributed to —
	// and signed by — the adopted identity rather than the session's login member.
	SetPlanetMember(planetID, memberID tag.UID)

	// Called after a new epoch key has been stored in EpochKeyStore.  Notifies
	// the vault controller to re-verify pending journal entries for this epoch.
	// See SetPlanet for the ordering contract this call closes.
	OnEpochKeyArrived(epochID tag.UID)

	// Processes a verified planet-public governance TxMsg (e.g. MemberEpoch distribution).
	// Called by the vault controller after signature verification succeeds.
	// Routes the TxMsg to all registered governance handlers for epoch key extraction.
	OnGovernanceTx(planetID tag.UID, tx *TxMsg)

	// journal introspection

	// Returns the local journal's high-water TxID and entry count for planetID —
	// metadata only (never plaintext), so it needs no epoch key.
	PlanetHighWater(planetID tag.UID) (highWater tag.UID, count int)

	// Returns this node's operator-configured home-vault endpoint(s), seeded into
	// home-planet governance (EpochTerms.VaultConfig.VaultAddrs) at genesis so a
	// peerless acceptor can dial onto the planet.  Empty when no vault is configured.
	VaultHomeAddrs() []string

	// GenesisEpoch reads planetID's genesis PlanetEpoch — the immutable
	// PlanetCharter (privacy mode, founder set, genesis quorum) frozen at
	// genesis — from the journal, the authoritative founder source: the
	// session's planet registry holds only a lightweight Terms-stub epoch for
	// restored or invite-joined planets.  Fails when the node does not hold
	// the planet.
	GenesisEpoch(planetID tag.UID) (*PlanetEpoch, error)

	// KeyAdmission returns the login boundary's member-signing-key custody
	// ruling for this session, computed once in the host's login handler
	// before app.home MakeReady (SD-security-sync §8.5).
	KeyAdmission() KeyAdmission

	// access control

	// ACC returns the host's access-control engine.
	ACC() ACCEngine
}

// KeyAdmission is the login boundary's ruling on member-signing-key custody
// for one session.  At most one of AdoptDeclared / MintNodeKey is acted on by
// app.home's key-ensure; when neither is set an unknown member is refused
// cleanly — the Enclave stays signing-keyless, the login challenge verify
// rejects, and nothing about the rejected first contact persists.  Both
// AdoptDeclared and ChallengeRequired derive from the same transport predicate
// in the login handler, so "adoption ⇒ verified proof-of-possession" holds
// structurally, never by flag convention (SD-security-sync §8.5).
type KeyAdmission struct {
	ChallengeRequired bool // a challenge is issued AND verified this login
	AdoptDeclared     bool // import the client-declared pubkey, pub-only
	MintNodeKey       bool // generate a node-held keypair (node-custodial only)
}

// ACCEngine is the host's access-control resolver: it answers "who may do what" from a
// planet's verified governance state (channel epochs + the immutable founder set).  A
// Session exposes it OFF the public amp.Session interface for first-party governance apps
// (members, home) to consult.  The host's ACC engine is the sole implementation.
// See AOM SD-channel-governance.md.
type ACCEngine interface {

	// ChannelEpoch returns the latest governing ChannelEpoch for (planetID, nodeID),
	// or nil when the channel is ungoverned.
	ChannelEpoch(planetID, nodeID tag.UID) *ChannelEpoch

	// HasAccess reports whether memberID holds at least `required` access on the channel.
	HasAccess(planetID, nodeID, memberID tag.UID, required Access) bool

	// ResolveAccess returns memberID's effective Access on the channel (parent-chain
	// resolved, fail-closed at any missing ancestor).
	ResolveAccess(planetID, nodeID, memberID tag.UID) Access

	// IsFounder reports whether memberID is a founder of planetID — PlanetCharter.Founders,
	// verified from the immutable genesis envelope (the root of governance authority).
	IsFounder(planetID, memberID tag.UID) bool

	// IsMember reports whether memberID holds an admitted MemberEpoch on planetID.
	IsMember(planetID, memberID tag.UID) bool

	// FounderFingerprint returns planetID's resolved founder fingerprint — the
	// commitment to its genesis founder authority root (FounderFingerprint fn;
	// SD-channel-governance §8) — or nil until the genesis resolves.
	FounderFingerprint(planetID tag.UID) []byte

	// PinFounderFingerprint registers the founder fingerprint planetID's
	// genesis is EXPECTED to match — carried in from a PlanetInvite or
	// NameServiceRecord before first sync.  The engine's founder scan skips a
	// genesis that mismatches a registered pin.  Empty is a no-op; a pin that
	// conflicts with an existing pin or an already-resolved fingerprint errors
	// (fail-closed; the first pin holds).
	PinFounderFingerprint(planetID tag.UID, expected []byte) error

	// InvitePolicy returns the latest admitted invite policy for inviteID on
	// planetID, or nil — the gated view app.invite reads and the invite ACC
	// rule enforces against.
	InvitePolicy(planetID, inviteID tag.UID) *PlanetInvitePolicy

	// InvitePolicies returns planetID's admitted invite policies, keyed by
	// invite ID.
	InvitePolicies(planetID tag.UID) map[tag.UID]*PlanetInvitePolicy

	// InviteRedemptions returns one invite's admitted redemption ledger, keyed
	// by RedeemedAt item ID.  Rank over it is the redemption count.
	InviteRedemptions(planetID, inviteID tag.UID) map[tag.UID]*PlanetInviteRedemption
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
	RangeHash(planetID tag.UID, start, end tag.UID) (tag.UID, error)

	// Quarantine rewrites the existing entry for (planetID, txTimeID) with a quarantine
	// flag and the given TTL.  Returns ErrNotFound-style status if the entry is absent.
	Quarantine(planetID tag.UID, txTimeID tag.UID, ttl time.Duration) error

	// ReadQuarantined iterates quarantined entries strictly after the `after` mark.
	// Parallel to ReadSince; returning false from the callback stops iteration.
	ReadQuarantined(planetID tag.UID, after tag.UID, cb func(txTimeID tag.UID, raw []byte) bool) error
}

// TxOutbox queues locally authored TxMsgs for propagation to vaults.  Entries persist across
// restarts — the outbox is drained when vault connectivity is available.  Blob bytes never
// queue: the relayed TxMsg is the announce, and receivers pull what its BlobRef ops name
// (receiver-driven transfer, SD-planet-storage §13.10).
type TxOutbox interface {
	EnqueueTx(planetID tag.UID, txTimeID tag.UID, raw []byte) error
	DrainTx(cb func(planetID tag.UID, txTimeID tag.UID, raw []byte) error) error
	Close() error
}

// BlobStore manages content-addressed encrypted blob storage.
// Blobs are identified by (PlanetID, BlobID) and validated by their content hash.
type BlobStore interface {

	// Store writes data under a caller-supplied blobID without deriving or validating it — the raw
	// primitive beneath StoreHashed (derives the ID from the plaintext hash) and StoreValidated
	// (validates the stream against an existing BlobTag.UID before publish).  Callers that already
	// hold the content address use it directly; most callers want StoreHashed or StoreValidated.
	Store(planetID tag.UID, blobID tag.UID, data io.Reader, byteSize int64) error
	Retrieve(planetID tag.UID, blobID tag.UID) (io.ReadCloser, error)
	Has(planetID tag.UID, blobID tag.UID) bool

	// StoreHashed hashes and stores planet-public data content-addressed in a single streaming pass.
	// Caller pre-populates ref.PlanetID_0/1, ref.HashKitID (0 = default Blake2s_256), and optionally
	// the asset identity ref.AssetTag.ContentType() / ref.AssetTag.Text.
	// On success, StoreHashed populates ref.Hash_0..3 from the content hash and both content
	// addresses (§13.2): the asset identity AssetTag (UID = leading 16 bytes of the plaintext hash,
	// I / Units = Bytes = authoritative plaintext byte count) and the lean storage identity BlobTag.
	// A public blob's stored bytes are the plaintext, so BlobTag.UID == AssetTag.UID.
	// Idempotent: if the hash-derived on-disk path already exists, the temp write is discarded.
	StoreHashed(ref *BlobRef, data io.Reader, onProgress func(bytesWritten int64)) error

	// StoreValidated streams a peer-supplied blob into a temp file under the planet dir, hashing it
	// with ref.HashKitID, and publishes it at the content-addressed path only when the streamed
	// bytes satisfy hash(stream)[:16] == ref.BlobTag.UID() — the address of the bytes as stored
	// (ciphertext for a sealed blob, plaintext for a public one), validated without the epoch key
	// (§13.2.1). This is the receiver's O(1)-memory ingest: no whole-blob buffer, atomic temp+rename
	// mirroring StoreHashed. On hash mismatch or I/O error the temp is discarded and an error
	// returned, so the durable store and any presence tracking are never poisoned by a partial or
	// invalid transfer. It validates against the existing BlobTag.UID and does not recompute the
	// asset identity (ref.Hash_0..3 / AssetTag — the member's concern, resolved from the sealed
	// TxMsg). Idempotent: an existing final path discards the temp and returns nil.
	StoreValidated(planetID tag.UID, ref *BlobRef, data io.Reader) error
}

// Registry is where apps and types are registered -- concurrency safe.
type Registry interface {

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
	Requester              // origin of this request
	Current   PinRequest   // merge-accumulated wire state (proto.Merge of every PinRequest revision)
	Selector  ItemSelector // normalized working copy of Current.Selector, rebuilt by Revise
	Tx        *TxMsg       // tx to process for this request
	ID        tag.UID      // universally unique ID for this request (inherited from tx invoking this request)
	InvokeURL *url.URL     // derived from PinRequest.URL in Request.Revise()
	Params    url.Values   // derived from PinRequest.URL in Request.Revise()
}

// CRDT kv entry pair
type ValueEntry struct {
	Addr  tag.Address   // CRDT value address
	Value proto.Message // initialized with default value of expected type
}

// Endpoint expresses a network protocol and address to bind / list / send to.
type Endpoint struct {
	Network string
	Address string
}
