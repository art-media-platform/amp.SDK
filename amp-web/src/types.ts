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
  DisplayName: string;
  Email?: string;        // present when the auth scheme exposes it
  PlanetID: string;      // planet tag.UID, base32
  Kind?: string;         // tag.UID resolving to a LawMemberKind_* (AOM substrate-agnostic-members)
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
}

export interface AmpAuth {
  member: AmpMember | null;
  isAuthenticated: boolean;
  loading: boolean;
  login: (credentials: LoginCredentials) => Promise<void>;
  logout: () => Promise<void>;
}

// ── Tag resolution (server canonization) ────────────────────────────

export interface TagResolution {
  Expr: string;
  Canonic: string;
  ID: string;            // base32 tag.UID
}

// ── Withdrawal & addresses (AOM withdrawal-consent / AOM cross-planet-citation) ──────────────────

export type WithdrawReason =
  | 'NoReason' | 'Consent' | 'Inaccuracy' | 'Outdated' | 'Coerced'
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

export interface AmpQueryOpts {
  itemID?: string;                      // fetch a single item by ID
  limit?: number;                       // page size (default: 50)
  after?: string;                       // cursor (itemID to start after)
  orderBy?: string;                     // client-side ordering hint
  filter?: Record<string, unknown>;     // client-side equality filter hint
  planetTag?: string;                   // read a planet other than the session default
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
  ContentType?: string;
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
    };
