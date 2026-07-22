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

// ChallengeResponse is the body of GET /api/v1/login/challenge?address=0x…
// (or ?did=…) — the server-issued personal-sign challenge the wallet and DID
// login schemes sign.  Message is the EIP-4361 payload (or the generic
// domain-bound message for a non-EVM DID); the follow-up LoginRequest echoes
// Nonce, and the challenge itself is server-stored keyed by Nonce, never
// resent.  The TS mirror is types.ts WalletChallenge.
type ChallengeResponse struct {
	Nonce     string `json:"Nonce"`
	Message   string `json:"Message"`
	ExpiresAt int64  `json:"ExpiresAt,omitempty"` // unix seconds — challenge nonce expiry
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
//   - Request body for all four endpoints (admin issue, recover, redeem,
//     account claim) is the proto webapi.EmailCredential (defined in
//     webapi.proto; decoded via encoding/json).
//   - EmailIssueResponse below echoes the seeded MemberID on /issue success.

// EmailIssueResponse echoes the seeded MemberID so the caller (admin tool /
// migration script) can record the wallet→email mapping without re-hashing.
type EmailIssueResponse struct {
	MemberID tag.UID `json:"MemberID"`
	Email    string  `json:"Email"`
}

// Operator tier (POST /api/v1/admin/*) — node-custodial verbs gated by the
// per-org admin allowlist, deliberately WITHOUT a browser-SDK binding: the
// operator Bearer is higher-privilege than a member session and must never
// normalize into XSS-exposed browser JS.  testdata/operator-go-only.json is
// the enforcing manifest — the TS drift test asserts the browser client has
// no binding for these verbs (a NodeAdminModule verb binds only in the SDK's
// Node-only admin module), so adding or relaxing a binding forces a reviewed
// manifest edit.
// PlanetCreate*/BrandSet* are proto messages (webapi.proto); EmailCredential /
// EmailIssueResponse (above) double as the issueEmailCredential bodies; the
// forums-reserve shapes below complete the tier.

// ForumsReserveRequest is the body of POST /api/v1/admin/forums/reserve —
// the admin allowlist row for an invite-only board.  Exactly one of Address
// (an Eth wallet address; the member UID derives server-side) or MemberID
// (base32 UID, any identity scheme).
type ForumsReserveRequest struct {
	Address  string `json:"Address,omitempty"`
	MemberID string `json:"MemberID,omitempty"`
}

// ForumsReserveResponse echoes the reserved member UID so the caller can
// persist the identity↔reservation mapping without re-hashing.
type ForumsReserveResponse struct {
	MemberID tag.UID `json:"MemberID"`
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

// ── Invites (§ app.invite) ──────────────────────────────────────────────
//
// Governed invites: an issuer mints a policy-bearing invite (single-use
// pre-minted slot, or multi-use self-mint with MaxRedemptions), a redeemer
// joins under it, and every redemption leaves a ledger record.  The sealed
// invite travels as InviteText — the universal URL https://{fqdn}/invite#… (or
// its bare amp-base32 body); the passphrase is always delivered out-of-band.

// InviteIssueRequest is the body of POST /api/v1/invite/issue (Bearer).
// MaxRedemptions == 0 mints a single-use pre-minted slot; > 0 mints a multi-use
// self-mint policy.  Access is the enum name a redeemer is granted ("ReadWrite",
// …); empty = the planet's default.  ExpiresAt is unix seconds; 0 = the planet's
// bootstrap TTL.
type InviteIssueRequest struct {
	Planet         string   `json:"Planet"`                   // amp-base32 UID of the planet to invite to
	Passphrase     string   `json:"Passphrase"`               // seals the returned invite (out-of-band from the URL)
	MaxRedemptions uint32   `json:"MaxRedemptions,omitempty"` // 0 = single-use; > 0 = multi-use ceiling
	Access         string   `json:"Access,omitempty"`         // access enum name granted per redeemer
	ExpiresAt      int64    `json:"ExpiresAt,omitempty"`      // unix seconds; 0 = planet bootstrap TTL
	VaultAddrs     []string `json:"VaultAddrs,omitempty"`     // optional bootstrap peer addresses
}

// InviteIssueResponse returns the sealed invite as its universal URL plus the
// invite's ID (the sealed-body hash — the ledger + revocation key).
type InviteIssueResponse struct {
	PlanetID   tag.UID `json:"PlanetID"`
	InviteID   tag.UID `json:"InviteID"`
	InviteText string  `json:"InviteText"` // the invite's universal URL (https://{fqdn}/invite#…)
}

// InviteAcceptRequest is the body of POST /api/v1/invite/accept (Bearer).
type InviteAcceptRequest struct {
	InviteText string `json:"InviteText"` // the invite URL or its amp-base32 body (transit noise tolerated)
	Passphrase string `json:"Passphrase"`
}

// InviteAcceptResponse returns the joined planet and the redeemer's member UID.
type InviteAcceptResponse struct {
	PlanetID tag.UID `json:"PlanetID"`
	MemberID tag.UID `json:"MemberID"`
}

// InviteRevokeRequest is the body of POST /api/v1/invite/revoke (Bearer).
// Identify the invite by InviteID (the sealed-body hash) or InviteText.  Revoke
// is terminal.  Rotate also rotates the planet epoch to retire the token-held
// key (node-custodial founder only).
type InviteRevokeRequest struct {
	Planet     string `json:"Planet"`               // amp-base32 UID of the planet (required)
	InviteID   string `json:"InviteID,omitempty"`   // amp-base32 invite ID
	InviteText string `json:"InviteText,omitempty"` // or the invite URL / body
	Rotate     bool   `json:"Rotate,omitempty"`     // also rotate the planet epoch
}

// InviteListResponse is the body of GET /api/v1/invite/list?planet= (Bearer):
// a planet's invite policies with their redemption ledgers.
type InviteListResponse struct {
	Policies []InvitePolicyEntry `json:"Policies"`
}

// InvitePolicyEntry is one invite policy with its ranked redemption ledger.
// Status rides as its enum name ("InviteActive" / "InviteRevoked").
type InvitePolicyEntry struct {
	InviteID       tag.UID                 `json:"InviteID"`
	MaxRedemptions uint32                  `json:"MaxRedemptions"`
	GrantedAccess  string                  `json:"GrantedAccess,omitempty"`
	Status         string                  `json:"Status"`
	ExpiresAt      int64                   `json:"ExpiresAt,omitempty"`
	Redemptions    []InviteRedemptionEntry `json:"Redemptions,omitempty"`
}

// InviteRedemptionEntry is one ledger record with its adjudicated rank; InRank
// is false for an over-rank (void) record.
type InviteRedemptionEntry struct {
	Member     tag.UID `json:"Member"`
	RedeemedAt int64   `json:"RedeemedAt"` // unix seconds
	Rank       int     `json:"Rank"`
	InRank     bool    `json:"InRank"`
}

// ── Governance (§ app.home ChannelEpoch) ────────────────────────────────
//
// A ChannelEpoch is the authoritative governance record for its channel and is
// latest-wins-REPLACE (an op naming one member replaces the whole ACL), so the
// endpoint takes the COMPLETE desired policy.  Incremental single-member grants
// are a read-modify-set the full-participant (native) client does against its
// synced governance state; this façade does not synthesize them.

// GrantEntry is one access grant.  Member is a base32 member UID; leave it
// empty for a DefaultGrants entry (the grant that applies to members not
// named).  Access is an amp.Access enum name — the full vocabulary
// (testdata/access.json AccessLevels is the golden).
type GrantEntry struct {
	Member string `json:"Member,omitempty"`
	Access string `json:"Access"`
}

// GovernanceGrantRequest is the body of POST /api/v1/governance/grant — the
// complete access policy for one channel of a planet (Bearer; the caller
// administers the planet).
type GovernanceGrantRequest struct {
	Planet            string         `json:"Planet"`                  // base32 UID of the planet whose channel is governed
	Channel           string         `json:"Channel"`                 // canonic name or base32 UID of the channel node
	Parent            string         `json:"Parent,omitempty"`        // optional parent legislating channel (canonic-or-UID)
	MemberGrants      []GrantEntry   `json:"MemberGrants,omitempty"`  // per-member permissions
	DefaultGrants     []GrantEntry   `json:"DefaultGrants,omitempty"` // permissions for members not named in MemberGrants
	CitedAttestations []*amp.Address `json:"CitedAttestations,omitempty"`
}

// GovernanceGrantResponse is the committed policy's addressing.
type GovernanceGrantResponse struct {
	PlanetID tag.UID `json:"PlanetID"`
	Channel  tag.UID `json:"Channel"`
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
