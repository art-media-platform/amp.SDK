/**
 * Core types for @art-media-platform/web.
 *
 * These mirror the canonical wire contract in amp.SDK/amp/webapi
 * (webapi.types.go) in web-friendly JSON form.  Field names and JSON tags
 * match the Go structs one-to-one — the Go side is the spec.
 */

// ── Authentication ──────────────────────────────────────────────────

export interface AmpMember {
  id: string;            // member tag.UID, base32
  displayName: string;
  email?: string;        // present when the auth scheme exposes it
  planetID: string;      // planet tag.UID, base32
  kind?: string;         // tag.UID resolving to a LawMemberKind_* (DESIGN-11)
  address?: string;      // 0x-prefixed; present for wallet-scheme members
}

/** Discriminated union mirroring webapi.LoginRequest (the `scheme` field). */
export type LoginCredentials =
  | { scheme: 'wallet'; address: string; signature: string; nonce: string }
  | { scheme: 'email'; email: string; password: string }
  | { scheme: 'memberToken'; memberToken: string }
  | { scheme: 'yubikey'; challengeResponse: string };

/** The personal-sign challenge a wallet scheme signs before login. */
export interface WalletChallenge {
  nonce: string;
  message: string;
}

export interface AmpAuth {
  member: AmpMember | null;
  isAuthenticated: boolean;
  loading: boolean;
  login: (credentials: LoginCredentials) => Promise<void>;
  logout: () => Promise<void>;
}

// ── Tag resolution (server canonicalization) ────────────────────────

export interface TagResolution {
  expr: string;
  canonic: string;
  id: string;            // base32 tag.UID
}

// ── Withdrawal & citations (DESIGN-15 / DESIGN-12) ──────────────────

export type WithdrawReason =
  | 'Consent' | 'Inaccuracy' | 'Outdated' | 'Coerced'
  | 'Forgotten' | 'Departed' | 'InviteRecall' | 'Retracted';

/** A (planetID, nodeID, itemID) triple addressing a record across planets. */
export interface CitationRef {
  planetID?: string;     // omitted = same planet as the request
  nodeID?: string;
  itemID?: string;
}

export interface WithdrawNote {
  reason: WithdrawReason;
  rationale?: string;
  withdrawnAt: string;   // ISO-8601, server-observed
  withdrawnBy: string;   // signer's member UID
  subject?: string;      // whose consent is withdrawn (omitted = signer)
  delegation?: CitationRef;
}

// ── CRDT item metadata ──────────────────────────────────────────────

export interface AmpItemMeta {
  _itemID: string;
  _editID: string;
  _fromID: string;
  _updatedAt: string;        // ISO-8601, derived from the item's tag.UID
  _withdrawn?: WithdrawNote;  // present when a Withdraw cites this item
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

/** One CRDT op inside a /api/v1/tx batch. */
export interface TxOp {
  kind: TxOpKind;
  channel: string;
  attr: string;
  itemID?: string;
  value?: Record<string, unknown>;
  reason?: WithdrawReason;     // withdraw ops only
  rationale?: string;         // withdraw ops only
  subject?: string;           // withdraw ops only
  delegation?: CitationRef;   // withdraw ops only
}

export interface TxResult {
  itemID: string;
  editID: string;
  error?: string;
}

export interface WithdrawOpts {
  reason: WithdrawReason;
  rationale?: string;
  subject?: string;           // defaults to the signer when omitted
  delegation?: CitationRef;   // cites the record proving delegated authority
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

export interface BlobRef {
  id: string;              // blob tag.UID, base32
  streamURL?: string;      // /www/{id} on the vault — set by upload + resolve
  contentType?: string;
  byteSize?: number;
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

export type SubscriptionEvent =
  | { type: 'update'; itemID: string; value: Record<string, unknown>; editID: string; fromID: string; updatedAt?: string }
  | { type: 'delete'; itemID: string; editID?: string; fromID?: string }
  | {
      type: 'withdraw';
      itemID: string;
      editID?: string;
      fromID?: string;
      reason: WithdrawReason;
      rationale?: string;
      subject?: string;
      delegation?: CitationRef;
    };
