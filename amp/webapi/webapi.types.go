// Package webapi defines the wire-level JSON shapes for the amp /api/v1/*
// surface — the contract between the amp web SDK (@art-media-platform/web and
// other language bindings) and an amp host (ampd).
//
// Field names MUST match the wire shape one-to-one — every external SDK
// (TypeScript, C#, Swift, future Python) reflects against these names.  Do not
// rename without version-bumping the API surface.  JSON keys are PascalCase,
// matching the Go (and C#) identifiers — one identifier set across all
// platforms.  Two deliberate exceptions: the protocol-metadata keys on `Item`
// carry a leading `_` (`_ItemID`, `_EditID`, …) to stay clear of app data keys,
// and URL query params (`?after=`, `?limit=`, `?planetTag=`) are lowerCamelCase.
//
// Field-type discipline:
//   - tag.UID for fields that are ALWAYS UIDs (member IDs, item/edit/from
//     IDs, citation triples, blob IDs).  The custom MarshalJSON /
//     UnmarshalJSON on tag.UID encodes/decodes via base32 — string transport
//     carries UIDs as base32, binary transport carries the fixed64 pair.
//   - string for fields that may carry a canonic name OR a UID (channel,
//     attr, PlanetTag, withdrawal subject before resolution) and for fields
//     that are not UIDs at all (Bearer tokens, ISO timestamps, Eth
//     addresses, free text).
//   - omitzero (Go 1.24+) on optional tag.UID fields so a zero-UID
//     suppresses the JSON entry, matching the omitempty behaviour of "".
package webapi

import (
	"encoding/json"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// LoginRequest is the body of POST /api/v1/login.
//
// Scheme dispatches the credential parse — only one of the per-scheme fields
// will be populated.  This mirrors the discriminated union in
// `LoginCredentials` on the SDK side.
type LoginRequest struct {
	Scheme string `json:"Scheme"`

	// scheme = "wallet"  (Ethereum personal-sign / MetaMask)
	Address   string `json:"Address,omitempty"`
	Signature string `json:"Signature,omitempty"`
	Nonce     string `json:"Nonce,omitempty"`

	// scheme = "email"
	Email    string `json:"Email,omitempty"`
	Password string `json:"Password,omitempty"`

	// scheme = "memberToken"
	MemberToken string `json:"MemberToken,omitempty"`

	// scheme = "yubikey"
	ChallengeResponse string `json:"ChallengeResponse,omitempty"`

	// scheme = "did"  (W3C DID 1.0 — did:key / did:pkh).  DID carries the full
	// URI; the signature over the server-issued challenge reuses Signature +
	// Nonce.  The challenge itself is server-stored keyed by Nonce (never on
	// the wire), exactly like the wallet flow.
	DID string `json:"DID,omitempty"`

	// Optional planet binding requested by the caller.  Accepts a canonic
	// tag.Name expression OR a base32 UID.  Empty = the host's home planet.
	PlanetTag string `json:"PlanetTag,omitempty"`
}

// LoginResponse is the body of a successful POST /api/v1/login.
//
// SessionToken stays as string — it's a 32-byte random Bearer, not a UID.
type LoginResponse struct {
	SessionToken string    `json:"SessionToken"`
	ExpiresAt    int64     `json:"ExpiresAt"` // unix seconds
	Member       AmpMember `json:"Member"`
}

// AmpMember describes the authenticated identity on the wire.  `Kind` is a
// human-readable label (e.g. "Person") sourced from a `LawMemberKind_*`
// definition (AOM SD-substrate-agnostic-members.md); apps surface it but do not gate behavior on it.
type AmpMember struct {
	ID          tag.UID `json:"ID"`
	DisplayName string  `json:"DisplayName,omitempty"`
	Email       string  `json:"Email,omitempty"`
	PlanetID    tag.UID `json:"PlanetID"`
	Kind        string  `json:"Kind,omitempty"`
	Address     string  `json:"Address,omitempty"` // populated for wallet-scheme members
}

// SessionResponse is the body of GET /api/v1/session.
type SessionResponse struct {
	Member    AmpMember `json:"Member"`
	ExpiresAt int64     `json:"ExpiresAt"`
}

// Email credential request/response shapes:
//   - Request body for all three endpoints is the proto webapi.EmailCredential
//     (defined in webapi.proto; decoded via encoding/json).
//   - EmailIssueResponse below echoes the seeded MemberID on /issue success.

// EmailIssueResponse echoes the seeded MemberID so the caller (admin tool /
// migration script) can record the wallet→email mapping without re-hashing.
type EmailIssueResponse struct {
	MemberID tag.UID `json:"MemberID"`
	Email    string  `json:"Email"`
}

// WithdrawNote is the wire-shape carrier for AOM SD-withdrawal-consent.md withdrawal facts,
// carried in TxOp.Withdraw / EditEntry.Withdraw / Item.Withdrawn /
// SubscribeFrame.Withdraw.  It is never marshaled to binary, so its UID
// fields are typed tag.UID / amp.Address and ride the JSON wire as base32
// strings — consistent with every other webapi shape.
//
// Sender sets Reason/Rationale/Subject/Delegation; the server fills
// WithdrawnAt/WithdrawnBy on the response side.  Reason rides as its enum
// name ("Consent", …) via amp.WithdrawReason's marshaler.
type WithdrawNote struct {
	Reason      amp.WithdrawReason `json:"Reason"`
	Rationale   string             `json:"Rationale,omitempty"`
	Subject     tag.UID            `json:"Subject,omitzero"`      // signer is the implicit subject when zero
	Delegation  *amp.Address       `json:"Delegation,omitempty"`  // nil when Subject == signer
	WithdrawnAt string             `json:"WithdrawnAt,omitempty"` // ISO-8601 (response-side; sender leaves empty)
	WithdrawnBy tag.UID            `json:"WithdrawnBy,omitzero"`  // response-side; sender leaves empty
}

// TxOpKind enumerates the verbs accepted in a /api/v1/tx batch.
type TxOpKind string

const (
	TxOpCreate   TxOpKind = "create"
	TxOpUpsert   TxOpKind = "upsert"
	TxOpRemove   TxOpKind = "remove"
	TxOpWithdraw TxOpKind = "withdraw"
)

// TxOp is one CRDT operation inside a /api/v1/tx batch.
//
// Channel/Attr/ItemID stay as string because callers may pass either the
// canonic name OR a pre-resolved base32 UID (the server runs them through
// tag.Parse).
//
// `Value` is held as raw JSON until the backend marshals it to the
// registered proto for the (channel, attr) pair — late binding lets
// unfamiliar attrs still round-trip through the wire layer without
// server-side schema knowledge.
//
// Withdraw is non-nil only on withdraw ops (Kind = TxOpWithdraw); it carries
// the AOM SD-withdrawal-consent.md facts (Reason/Rationale/Subject/Delegation).  Non-nil on a
// non-withdraw op is ignored.
type TxOp struct {
	Kind     TxOpKind        `json:"Kind"`
	Channel  string          `json:"Channel"`
	Attr     string          `json:"Attr"`
	ItemID   string          `json:"ItemID,omitempty"`
	Value    json.RawMessage `json:"Value,omitempty"`
	Withdraw *WithdrawNote   `json:"Withdraw,omitempty"`
}

// TxRequest is the body of POST /api/v1/tx.
type TxRequest struct {
	Ops []TxOp `json:"Ops"`

	// PlanetTag is optional; defaults to the session's bound planet.  Lets a
	// caller direct a write at the deploy's share planet without
	// instantiating a second client.  Accepts canonic name or base32 UID.
	PlanetTag string `json:"PlanetTag,omitempty"`

	// InvokeURL, when set, routes the whole batch to an app verb handler instead
	// of the default cabinet-commit path: the host delivers the ops to the named
	// verb's StartPin as RPC arguments (PinMode_Invoke — not journaled as planet
	// state), carrying the session member as the tx FromID, and the app authors any
	// durable writes itself (custodially).  Form: "amp://~/{app}/{verb}" (e.g.
	// "amp://~/forums/post").  Empty = a normal cabinet commit.  One batch = one verb.
	InvokeURL string `json:"InvokeURL,omitempty"`
}

// TxOpResult is the per-op outcome inside a TxResponse.
type TxOpResult struct {
	ItemID tag.UID `json:"ItemID"`
	EditID tag.UID `json:"EditID"`
	Error  string  `json:"Error,omitempty"`
}

// TxResponse is the body of POST /api/v1/tx on success.
type TxResponse struct {
	TxID    tag.UID      `json:"TxID"`
	Results []TxOpResult `json:"Results"`
}

// Item is one CRDT entry on the wire.  Underscore-prefixed fields surface the
// protocol's metadata (item / edit / from IDs, server-stamp time) alongside
// the application value — the `_` sigil keeps them clear of app data keys.
type Item struct {
	ItemID    tag.UID         `json:"_ItemID"`
	EditID    tag.UID         `json:"_EditID"`
	FromID    tag.UID         `json:"_FromID"`
	UpdatedAt string          `json:"_UpdatedAt"` // ISO-8601, derived from ItemID's tag.UID
	Value     json.RawMessage `json:"Value"`
	Withdrawn *WithdrawNote   `json:"_Withdrawn,omitempty"`
}

// ListResponse is the body of GET /api/v1/channels/:ch/attrs/:attr/items.
//
// Next is a cursor (the last item's base32 UID, or empty when no more
// pages); kept as string because the cursor is opaque from the client's
// point of view — callers pass it back verbatim via the `?after=` query.
type ListResponse struct {
	Items   []Item `json:"Items"`
	HasMore bool   `json:"HasMore"`
	Next    string `json:"Next,omitempty"`
}

// EditOp names what a chronicle entry did to its item.  Distinct from
// TxOpKind because the chronicle view collapses Create+Upsert into a single
// observed semantic (a record either appeared or was updated; the wire
// caller does not need to know whether the client called create vs upsert).
type EditOp string

const (
	EditOpUpsert   EditOp = "upsert"
	EditOpDelete   EditOp = "delete"
	EditOpWithdraw EditOp = "withdraw"
)

// EditEntry is one record in the chronicle replay of an item.
//
// Body carries the op's payload bytes (post-extraction; for upserts this is
// the marshaled amp.Tag with the JSON body, for withdraws this is the
// marshaled amp.Withdraw companion).  Kept opaque on the wire layer per the
// thin-wire posture — the caller decodes against the registered proto
// for the addressed attr.
//
// Withdraw is non-nil only on withdraw entries (Op = EditOpWithdraw).
type EditEntry struct {
	EditID      tag.UID         `json:"EditID"`
	CommitTx    tag.UID         `json:"CommitTx"`
	Author      tag.UID         `json:"Author"`
	CommittedAt string          `json:"CommittedAt"` // ISO-8601, derived from CommitTx UID
	Op          EditOp          `json:"Op"`
	Withdraw    *WithdrawNote   `json:"Withdraw,omitempty"`
	Body        json.RawMessage `json:"Body,omitempty"`
}

// EditChainResponse is the body of GET /api/v1/channels/:ch/attrs/:attr/items/:itemID/edits.
//
// Original is the first chronicled record on the item (the Add).  Edits is
// the full replay in commit order, INCLUDING the original entry as its
// first element, so an auditor can iterate one slice.  Sealed in for the
// v300 wire freeze; new entry kinds extend EditOp without a shape change.
type EditChainResponse struct {
	Original *Item       `json:"Original,omitempty"`
	Edits    []EditEntry `json:"Edits"`
}

// MediaResolveRequest is the body of POST /api/v1/media/resolve.
//
// Blob is an amp.Tag carrying the blob's identity + metadata:
//   - Tag.UID: blob ID (leading 16 bytes of plaintext hash), base32
//   - Tag.ContentType(): MIME type
//   - Tag.I + Tag.Units (= Bytes): plaintext byte length
//   - Tag.URI: server-populated stream URL on response; ignored on request
//
// Caller-carries-the-Tag posture: blob metadata lives in the cabinet, not
// in any wire-side persistent store.  The publisher is in-memory and
// idempotent — the same blob always resolves to the same URL on a given
// host, and a vault outage is recoverable by republishing on whichever
// host the SDK reaches next.
type MediaResolveRequest struct {
	PlanetTag string   `json:"PlanetTag,omitempty"`
	Blob      *amp.Tag `json:"Blob"`
}

// SubscribeFrame is the WebSocket fan-out shape for /ws.  Clients send
// {Type:"subscribe"|"unsubscribe", Channel, Attr}; the server pushes
// {Type:"update"|"delete"|"withdraw", Channel, Attr, ItemID, Value?, EditID?, FromID?}.
//
// Channel + Attr stay as string for the same canonic-or-UID reason as TxOp.
// ItemID/EditID/FromID are typed UIDs.
//
// Withdraw is non-nil on withdraw frames (Type = "withdraw") and carries the
// full AOM SD-withdrawal-consent.md record so subscribers can reconstruct it without a
// follow-up read.
type SubscribeFrame struct {
	Type      string          `json:"Type"`
	Channel   string          `json:"Channel,omitempty"`
	Attr      string          `json:"Attr,omitempty"`
	ItemID    tag.UID         `json:"ItemID,omitzero"`
	EditID    tag.UID         `json:"EditID,omitzero"`
	FromID    tag.UID         `json:"FromID,omitzero"`
	Value     json.RawMessage `json:"Value,omitempty"`
	UpdatedAt string          `json:"UpdatedAt,omitempty"`
	Withdraw  *WithdrawNote   `json:"Withdraw,omitempty"`
	Error     string          `json:"Error,omitempty"`
}

// NameService / federation directory shapes — POST /api/v1/resolve,
// POST /api/v1/search, GET /api/v1/federation/peers.
//
// These mirror the substrate resolver (nameservice.Resolution / Match /
// amp.FederationPeer).  POST /api/v1/resolve is anonymous: exact-match resolution
// answers off the host's federation resolver and returns VaultAddrs in full so any
// caller — a fresh install, a deep-link source — can dial + pin the named planet
// (FQDN keys are low-entropy and dictionary-reversible, so namespace privacy comes
// from federation unreachability, not key secrecy).  Search and federation/peers
// require Bearer auth: ranked enumeration is the scraping surface, so a session
// walks only the federations it has joined.

// ResolveRequest is the body of POST /api/v1/resolve.
type ResolveRequest struct {
	FQDN string `json:"FQDN"`
}

// ResolveResponse is an exact-match FQDN resolution.  TrustState rides as its
// enum name ("Verified", …) via amp.TrustState's marshaler; a consumer must
// not silently follow a non-Verified or Ambiguous answer.
type ResolveResponse struct {
	FQDN          string          `json:"FQDN"`
	PlanetID      tag.UID         `json:"PlanetID"`
	AnsweredBy    tag.UID         `json:"AnsweredBy"`
	VaultAddrs    []VaultEndpoint `json:"VaultAddrs,omitempty"`
	TrustState    amp.TrustState  `json:"TrustState"`
	PinPrecedence bool            `json:"PinPrecedence"`
	Ambiguous     bool            `json:"Ambiguous"`
	Hops          int             `json:"Hops"`
}

// VaultEndpoint is the JSON form of amp.VaultAddr — where a planet's vault is
// dialable.  Address rides as base64 (Go's default []byte JSON encoding).
type VaultEndpoint struct {
	Transport string `json:"Transport"`
	Address   []byte `json:"Address"`
}

// SearchRequest is the body of POST /api/v1/search.
type SearchRequest struct {
	Query string `json:"Query"`
	Limit int    `json:"Limit,omitempty"`
}

// SearchResponse is a ranked, best-effort search over cached federation
// records — membership-gated discovery, not a public directory dump.
type SearchResponse struct {
	Matches []SearchMatch `json:"Matches"`
}

// SearchMatch is one ranked result (mirrors nameservice.Match + Snippet).
type SearchMatch struct {
	PlanetID   tag.UID  `json:"PlanetID"`
	FQDN       string   `json:"FQDN"`
	AnsweredBy tag.UID  `json:"AnsweredBy"`
	Score      float64  `json:"Score"`
	AppName    string   `json:"AppName"`
	AppDesc    string   `json:"AppDesc"`
	Platforms  []string `json:"Platforms,omitempty"`
}

// FederationPeersResponse is the body of GET /api/v1/federation/peers.
type FederationPeersResponse struct {
	Peers []FederationPeerEntry `json:"Peers"`
}

// FederationPeerEntry is the JSON form of amp.FederationPeer — a peer / parent
// pointer a federation enumerates for cross-federation forwarding.
type FederationPeerEntry struct {
	FederationID tag.UID         `json:"FederationID"`
	VaultAddrs   []VaultEndpoint `json:"VaultAddrs,omitempty"`
	Label        string          `json:"Label,omitempty"`
}

// ErrorResponse is the body of every non-2xx /api/v1/* response.
type ErrorResponse struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

// Error codes are the client-facing error vocabulary carried on the
// ErrorResponse envelope; the HTTP status rides the response itself.  They are
// an HTTP-boundary projection of the transport-agnostic status.Code, not a
// mirror of it — the substrate keeps its granular diagnostic codes; this set is
// what an HTTP/SDK consumer dispatches on.
const (
	ErrBadRequest      = "BadRequest"
	ErrAuthRequired    = "AuthRequired"
	ErrAuthFailed      = "AuthFailed"
	ErrForbidden       = "Forbidden"
	ErrNotFound        = "NotFound"
	ErrConflict        = "Conflict"
	ErrUnsupported     = "Unsupported"
	ErrTxRejected      = "TxRejected"
	ErrPayloadTooLarge = "PayloadTooLarge"
	ErrInternal        = "Internal"
	ErrUnimplemented   = "Unimplemented"
)
