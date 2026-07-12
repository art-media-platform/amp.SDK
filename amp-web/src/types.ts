/**
 * Core types for @art-media-platform/web.
 *
 * Two casing conventions, by design:
 *   - WIRE DATA types (serialized to/from the /api/v1/* JSON) use PascalCase
 *     keys — one identifier set across Go, C#, and TypeScript (the Go side is
 *     the spec, amp.SDK/amp/webapi).  UIDs ride as base32 strings.
 *   - SDK ERGONOMIC types (option bags, hook return shapes, callbacks) use
 *     camelCase — they never serialize; this keeps the React surface idiomatic.
 */

// ── Authentication ──────────────────────────────────────────────────

export interface AmpMember {
  ID: string;            // member tag.UID, base32
  DisplayName?: string;
  Email?: string;        // present when the auth scheme exposes it
  PlanetID: string;      // planet tag.UID, base32
  Kind?: string;         // tag.UID resolving to a LawMemberKind_* (Person / Group / Agent / Memorial)
  Address?: string;      // 0x-prefixed; present for wallet-scheme members
}

/**
 * Discriminated union mirroring webapi.LoginRequest.  The `Scheme` key is
 * PascalCase like every wire field; the scheme VALUES ('wallet', 'email', …)
 * stay lowercase — the server dispatches on them verbatim.
 */
export type LoginCredentials =
  | { Scheme: 'wallet'; Address: string; Signature: string; Nonce: string }
  | { Scheme: 'email'; Email: string; Password: string }
  | { Scheme: 'memberToken'; MemberToken: string }
  | { Scheme: 'yubikey'; ChallengeResponse: string }
  | { Scheme: 'did'; DID: string; Signature: string; Nonce: string };

/**
 * EmailCredential is the shared request body for the email-credential
 * endpoints — one shape, three endpoints, each consumes a subset:
 *   - POST /api/v1/admin/credentials/email/issue   Email + Password
 *   - POST /api/v1/login/email/recover             Email
 *   - POST /api/v1/login/email/redeem              Token + NewPassword (+ PlanetTag)
 */
export interface EmailCredential {
  Email?: string;
  Password?: string;
  Token?: string;
  NewPassword?: string;
  PlanetTag?: string;
}

/** The personal-sign challenge a wallet/DID scheme signs before login. */
export interface WalletChallenge {
  Nonce: string;
  Message: string;
  ExpiresAt?: number;   // unix seconds — when the challenge nonce expires (server-set)
}

/** The host-validated session — body of GET /api/v1/session (webapi.SessionResponse). */
export interface AmpSession {
  Member: AmpMember;
  ExpiresAt: number;     // unix seconds
}

export interface AmpAuth {
  member: AmpMember | null;
  isAuthenticated: boolean;
  /** True while a login/logout call OR the initial session restore is in flight. */
  loading: boolean;
  /** True only during the initial restoreSession() pass — gate the login screen on this to avoid a signed-out flash on reload. */
  restoring: boolean;
  login: (credentials: LoginCredentials) => Promise<AmpMember>;
  logout: () => Promise<void>;
}

// ── Invites ─────────────────────────────────────────────────────────
//
// Governed invites: an issuer mints a policy-bearing invite (single-use
// pre-minted slot, or multi-use self-mint with a redemption ceiling), a
// redeemer joins under it, and every redemption leaves a ledger record.  The
// sealed invite travels as `inviteText` — the universal URL
// `https://{fqdn}/invite#…` (or its bare amp-base32 body); the passphrase is
// always delivered out-of-band, so the token is inert without it.

/** Options for issuing an invite (SDK ergonomic shape, camelCase). */
export interface InviteIssueOpts {
  planet: string;               // base32 UID of the planet to invite to
  passphrase: string;           // seals the returned invite (delivered out-of-band)
  maxRedemptions?: number;      // 0 / omitted = single-use pre-minted slot; > 0 = multi-use ceiling
  access?: AccessLevel;         // access each redeemer is granted; omitted = planet default
  expiresAt?: number;           // unix seconds; omitted = planet bootstrap TTL
  vaultAddrs?: string[];        // optional bootstrap peer addresses
}

/** Result of issuing an invite — the invite ID + its universal-URL text. */
export interface InviteIssueResult {
  PlanetID: string;
  InviteID: string;
  InviteText: string;
}

/** Options for redeeming a sealed invite. */
export interface InviteAcceptOpts {
  inviteText: string;           // the invite URL or its amp-base32 body
  passphrase: string;
}

/** Result of accepting an invite — the joined planet + this member, base32 UIDs. */
export interface InviteAcceptResult {
  PlanetID: string;
  MemberID: string;
}

/** Options for revoking an invite (terminal). */
export interface InviteRevokeOpts {
  planet: string;               // base32 UID of the planet
  inviteId?: string;            // base32 invite ID …
  inviteText?: string;          // … or the invite URL / body
  rotate?: boolean;             // also rotate the planet epoch (node-custodial founder only)
}

/** Access levels a redeemer may be granted (enum names, per the wire contract). */
export type AccessLevel =
  | 'ReadOnly' | 'ReadWrite' | 'Moderator' | 'Admin';

/** One invite policy with its rank-adjudicated redemption ledger. */
export interface InvitePolicyEntry {
  InviteID: string;
  MaxRedemptions: number;
  GrantedAccess?: string;
  Status: 'InviteActive' | 'InviteRevoked';
  ExpiresAt?: number;
  Redemptions?: InviteRedemptionEntry[];
}

/** One ledger record; `InRank` is false for an over-rank (void) record. */
export interface InviteRedemptionEntry {
  Member: string;
  RedeemedAt: number;           // unix seconds
  Rank: number;
  InRank: boolean;
}

/** Result of listing a planet's invites. */
export interface InviteListResult {
  Policies: InvitePolicyEntry[];
}

// ── Tag resolution (server canonization) ────────────────────────────

export interface TagResolution {
  Expr: string;
  Canonic: string;
  ID: string;            // base32 tag.UID
}

// ── Withdrawal & addresses ──────────────────────────────────────────

export type WithdrawReason =
  | 'Consent' | 'Inaccuracy' | 'Outdated' | 'Coerced'
  | 'Forgotten' | 'Departed' | 'InviteRecall' | 'Retracted';

/**
 * An Address points at a CRDT cell, optionally across planets.  On the wire
 * it is a single base32 string packing 3–5 UIDs (element / +edit / +planet)
 * — one token, one decode.  Treat it as opaque: the SDK passes through the
 * string the server produced.
 */
export type Address = string;

export interface WithdrawNote {
  Reason: WithdrawReason;
  Rationale?: string;
  WithdrawnAt?: string;  // ISO-8601, server-observed (response only)
  WithdrawnBy?: string;  // signer's member UID, base32 (response only)
  Subject?: string;      // whose consent is withdrawn, base32 (omitted = signer)
  Delegation?: Address;  // base32 packed Address proving delegated authority
}

// ── CRDT item metadata ──────────────────────────────────────────────

export interface AmpItemMeta {
  _ItemID: string;
  _EditID: string;
  _FromID: string;
  _UpdatedAt: string;         // ISO-8601, derived from the item's tag.UID
  _Withdrawn?: WithdrawNote;  // present when a Withdraw cites this item
}

// ── Query ───────────────────────────────────────────────────────────

/**
 * Query options.  amp is address-is-query: scope by (channel, attr), page by
 * the server-enforced ItemID window (`after`/`limit`).  There is deliberately
 * no server-side orderBy/filter — see SKILL "Address, don't filter"; name any
 * client-side view transform presentationally (sortView/searchView).
 */
export interface AmpQueryOpts {
  itemID?: string;                      // fetch a single item by ID
  limit?: number;                       // page size (default: 50)
  after?: string;                       // cursor (itemID to start after)
  planetTag?: string;                   // per-call planet; overrides the client's constructor default
}

export interface AmpQueryResult<T> {
  data: (T & AmpItemMeta)[];
  loading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
  hasMore: boolean;
  loadMore: () => Promise<void>;
}

// ── Mutation ────────────────────────────────────────────────────────

export type TxOpKind = 'create' | 'upsert' | 'remove' | 'withdraw';

/**
 * One CRDT op inside a /api/v1/tx batch (wire shape, PascalCase).
 *
 * For withdraw ops, populate `Withdraw` (a WithdrawNote sub-object) with
 * Reason/Rationale/Subject/Delegation.  Non-nil = active variant.
 */
export interface TxOp {
  Kind: TxOpKind;
  Channel: string;
  Attr: string;
  ItemID?: string;
  Value?: Record<string, unknown>;
  Withdraw?: WithdrawNote;     // withdraw ops only
}

export interface TxResult {
  ItemID: string;
  EditID: string;
  Error?: string;
}

export interface WithdrawOpts {
  reason: WithdrawReason;
  rationale?: string;
  subject?: string;           // base32 member UID; defaults to the signer when omitted
  delegation?: Address;       // base32 Address of the record proving delegated authority
}

export interface AmpMutationResult {
  /** Canonical batched write — one TxMsg, N ops, one signature. */
  tx: (ops: TxOp[], planetTag?: string) => Promise<TxResult[]>;
  /** Invoke an app verb — ops routed to verbURL's handler, member-authored. */
  invoke: (verbURL: string, ops: TxOp[], planetTag?: string) => Promise<TxResult[]>;
  create: (channel: string, attr: string, value: Record<string, unknown>) => Promise<string>;
  upsert: (channel: string, attr: string, itemID: string, value: Record<string, unknown>) => Promise<void>;
  remove: (channel: string, attr: string, itemID: string) => Promise<void>;
  withdraw: (channel: string, attr: string, itemID: string, opts: WithdrawOpts) => Promise<void>;
  loading: boolean;
  error: Error | null;
}

// ── Media / Blobs ───────────────────────────────────────────────────

/**
 * BlobRef mirrors the amp.Tag the server returns from /upload and
 * /media/resolve.  UID is the blob's base32 tag.UID; URI is the stream URL
 * (server-populated on resolve); I carries the plaintext byte length when
 * Units = Bytes.
 */
export interface BlobRef {
  UID: string;             // blob tag.UID, base32
  URI?: string;            // /www/{UID} stream URL — set by upload + resolve
  ContentTypeRaw?: string;
  I?: number;              // plaintext byte length (when Units = Bytes)
  Units?: number;
}

export interface UploadOpts {
  attr?: string;                              // attr to associate (optional)
  planetTag?: string;                         // target planet (optional)
  metadata?: Record<string, unknown>;
  onProgress?: (pct: number) => void;
}

export interface AmpUploadResult {
  upload: (file: File, channel: string, opts?: UploadOpts) => Promise<BlobRef>;
  progress: number;
  uploading: boolean;
  error: Error | null;
}

export interface AmpMediaResult {
  url: string | null;
  loading: boolean;
  contentType: string | null;
  byteSize: number | null;
  error: Error | null;
}

// ── Subscription events ─────────────────────────────────────────────
//
// Decoded from the flat webapi.SubscribeFrame the server pushes over /ws.
// Data identifiers stay PascalCase (wire-derived); `type` is the union
// discriminant.

export type SubscriptionEvent =
  | { type: 'update'; ItemID: string; Value: Record<string, unknown>; EditID: string; FromID: string; UpdatedAt?: string }
  | { type: 'delete'; ItemID: string; EditID?: string; FromID?: string }
  | {
      type: 'withdraw';
      ItemID: string;
      EditID?: string;
      FromID?: string;
      Withdraw: WithdrawNote;
    }
  // A server-side subscribe rejection (e.g. no access to the channel/attr) or a
  // malformed frame.  Routed to the (channel, attr) subscribers so a failed
  // subscription surfaces instead of silently never delivering.
  | { type: 'error'; Channel?: string; Attr?: string; Error: string };
