// Package webapi defines the wire-level JSON shapes for the amp /api/v1/*
// surface — the contract between the amp web SDK (@amp/web and other language
// bindings) and an amp host (amp.exe).
//
// Field names MUST match the wire shape one-to-one — every external SDK
// (TypeScript, C#, Swift, future Python) reflects against these names.  Do not
// rename without version-bumping the API surface.
//
// Field-type discipline:
//   - tag.UID for fields that are ALWAYS UIDs (member IDs, item/edit/from
//     IDs, citation triples, blob IDs).  The custom MarshalJSON /
//     UnmarshalJSON on tag.UID encodes/decodes via base32 — wire shape is
//     exactly the same string the previous string-typed surface produced.
//   - string for fields that may carry a canonic name OR a UID (channel,
//     attr, planetTag, withdrawal subject before resolution) and for fields
//     that are not UIDs at all (Bearer tokens, ISO timestamps, Eth
//     addresses, free text).
//   - omitzero (Go 1.24+) on optional tag.UID fields so a zero-UID
//     suppresses the JSON entry, matching the previous omitempty behaviour
//     of "" strings.
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
	Scheme string `json:"scheme"`

	// scheme = "wallet"  (Ethereum personal-sign / MetaMask)
	Address   string `json:"address,omitempty"`
	Signature string `json:"signature,omitempty"`
	Nonce     string `json:"nonce,omitempty"`

	// scheme = "email"
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`

	// scheme = "memberToken"
	MemberToken string `json:"memberToken,omitempty"`

	// scheme = "yubikey"
	ChallengeResponse string `json:"challengeResponse,omitempty"`

	// scheme = "did"  (W3C DID 1.0 — did:key / did:pkh).  DID carries the full
	// URI; the signature over the server-issued challenge reuses Signature +
	// Nonce.  The challenge itself is server-stored keyed by Nonce (never on
	// the wire), exactly like the wallet flow.
	DID string `json:"did,omitempty"`

	// Optional planet binding requested by the caller.  Accepts a canonic
	// tag.Name expression OR a base32 UID.  Empty = the host's home planet.
	PlanetTag string `json:"planetTag,omitempty"`
}

// LoginResponse is the body of a successful POST /api/v1/login.
//
// SessionToken stays as string — it's a 32-byte random Bearer, not a UID.
type LoginResponse struct {
	SessionToken string    `json:"sessionToken"`
	ExpiresAt    int64     `json:"expiresAt"` // unix seconds
	Member       AmpMember `json:"member"`
}

// AmpMember describes the authenticated identity on the wire.  `Kind` is a
// human-readable label (e.g. "Person") sourced from a `LawMemberKind_*`
// definition (DESIGN-11); apps surface it but do not gate behavior on it.
type AmpMember struct {
	ID          tag.UID `json:"id"`
	DisplayName string  `json:"displayName,omitempty"`
	Email       string  `json:"email,omitempty"`
	PlanetID    tag.UID `json:"planetID"`
	Kind        string  `json:"kind,omitempty"`
	Address     string  `json:"address,omitempty"` // populated for wallet-scheme members
}

// SessionResponse is the body of GET /api/v1/session.
type SessionResponse struct {
	Member    AmpMember `json:"member"`
	ExpiresAt int64     `json:"expiresAt"`
}

// Email credential request/response shapes:
//   - Request body for all three endpoints is the proto webapi.EmailCredential
//     (defined in webapi.proto; decoded via protojson).
//   - EmailIssueResponse below echoes the seeded MemberID on /issue success.

// EmailIssueResponse echoes the seeded MemberID so the caller (admin tool /
// migration script) can record the wallet→email mapping without re-hashing.
type EmailIssueResponse struct {
	MemberID tag.UID `json:"memberID"`
	Email    string  `json:"email"`
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
// the DESIGN-15 facts (Reason/Rationale/Subject/Delegation).  Non-nil on a
// non-withdraw op is ignored.
type TxOp struct {
	Kind     TxOpKind        `json:"kind"`
	Channel  string          `json:"channel"`
	Attr     string          `json:"attr"`
	ItemID   string          `json:"itemID,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"`
	Withdraw *WithdrawNote   `json:"withdraw,omitempty"`
}

// TxRequest is the body of POST /api/v1/tx.
type TxRequest struct {
	Ops []TxOp `json:"ops"`

	// PlanetTag is optional; defaults to the session's bound planet.  Lets a
	// caller direct a write at the deploy's share planet without
	// instantiating a second client.  Accepts canonic name or base32 UID.
	PlanetTag string `json:"planetTag,omitempty"`
}

// TxOpResult is the per-op outcome inside a TxResponse.
type TxOpResult struct {
	ItemID tag.UID `json:"itemID"`
	EditID tag.UID `json:"editID"`
	Error  string  `json:"error,omitempty"`
}

// TxResponse is the body of POST /api/v1/tx on success.
type TxResponse struct {
	TxID    tag.UID      `json:"txID"`
	Results []TxOpResult `json:"results"`
}

// Item is one CRDT entry on the wire.  Underscore-prefixed fields surface the
// protocol's metadata (item / edit / from IDs, server-stamp time) alongside
// the application value.
type Item struct {
	ItemID    tag.UID         `json:"_itemID"`
	EditID    tag.UID         `json:"_editID"`
	FromID    tag.UID         `json:"_fromID"`
	UpdatedAt string          `json:"_updatedAt"` // ISO-8601, derived from ItemID's tag.UID
	Value     json.RawMessage `json:"value"`
	Withdrawn *WithdrawNote   `json:"_withdrawn,omitempty"`
}

// ListResponse is the body of GET /api/v1/channels/:ch/attrs/:attr/items.
//
// Next is a cursor (the last item's base32 UID, or empty when no more
// pages); kept as string because the cursor is opaque from the client's
// point of view — callers pass it back verbatim via the `?after=` query.
type ListResponse struct {
	Items   []Item `json:"items"`
	HasMore bool   `json:"hasMore"`
	Next    string `json:"next,omitempty"`
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
// the marshaled std.TextItem with the JSON body, for withdraws this is the
// marshaled amp.Withdraw companion).  Kept opaque on the wire layer per the
// thin-wire posture — the caller decodes against the registered proto
// for the addressed attr.
//
// Withdraw is non-nil only on withdraw entries (Op = EditOpWithdraw).
type EditEntry struct {
	EditID      tag.UID         `json:"editID"`
	CommitTx    tag.UID         `json:"commitTx"`
	Author      tag.UID         `json:"author"`
	CommittedAt string          `json:"committedAt"` // ISO-8601, derived from CommitTx UID
	Op          EditOp          `json:"op"`
	Withdraw    *WithdrawNote   `json:"withdraw,omitempty"`
	Body        json.RawMessage `json:"body,omitempty"`
}

// EditChainResponse is the body of GET /api/v1/channels/:ch/attrs/:attr/items/:itemID/edits.
//
// Original is the first chronicled record on the item (the Add).  Edits is
// the full replay in commit order, INCLUDING the original entry as its
// first element, so an auditor can iterate one slice.  Sealed in for the
// v300 wire freeze; new entry kinds extend EditOp without a shape change.
type EditChainResponse struct {
	Original *Item       `json:"original,omitempty"`
	Edits    []EditEntry `json:"edits"`
}

// MediaResolveRequest is the body of POST /api/v1/media/resolve.
//
// Blob is an amp.Tag carrying the blob's identity + metadata:
//   - Tag.UID_0/UID_1: blob ID (leading 16 bytes of plaintext hash)
//   - Tag.ContentType: MIME type
//   - Tag.I + Tag.Units (= Bytes): plaintext byte length
//   - Tag.URI: server-populated stream URL on response; ignored on request
//
// Caller-carries-the-Tag posture: blob metadata lives in the cabinet, not
// in any wire-side persistent store.  The publisher is in-memory and
// idempotent — the same blob always resolves to the same URL on a given
// host, and a vault outage is recoverable by republishing on whichever
// host the SDK reaches next.
type MediaResolveRequest struct {
	PlanetTag string   `json:"planetTag,omitempty"`
	Blob      *amp.Tag `json:"blob"`
}

// SubscribeFrame is the WebSocket fan-out shape for /ws.  Clients send
// {type:"subscribe"|"unsubscribe", channel, attr}; the server pushes
// {type:"update"|"delete"|"withdraw", channel, attr, itemID, value?, editID?, fromID?}.
//
// Channel + Attr stay as string for the same canonic-or-UID reason as TxOp.
// ItemID/EditID/FromID are typed UIDs.
//
// Withdraw is non-nil on withdraw frames (Type = "withdraw") and carries the
// full DESIGN-15 record so subscribers can reconstruct it without a
// follow-up read.
type SubscribeFrame struct {
	Type      string          `json:"type"`
	Channel   string          `json:"channel,omitempty"`
	Attr      string          `json:"attr,omitempty"`
	ItemID    tag.UID         `json:"itemID,omitzero"`
	EditID    tag.UID         `json:"editID,omitzero"`
	FromID    tag.UID         `json:"fromID,omitzero"`
	Value     json.RawMessage `json:"value,omitempty"`
	UpdatedAt string          `json:"updatedAt,omitempty"`
	Withdraw  *WithdrawNote   `json:"withdraw,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// ErrorResponse is the body of every non-2xx /api/v1/* response.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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
