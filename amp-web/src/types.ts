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
 * endpoints — one shape, four endpoints, each consumes a subset:
 *   - POST /api/v1/admin/credentials/email/issue   Email + Password
 *   - POST /api/v1/login/email/recover             Email
 *   - POST /api/v1/login/email/redeem              Token + NewPassword (+ PlanetTag)
 *   - POST /api/v1/account/claim                   Email + Token + NewPassword (+ PlanetTag)
 */
export interface EmailCredential {
  Email?: string;
  Password?: string;
  Token?: string;
  NewPassword?: string;
  PlanetTag?: string;
}

/** Options for redeeming an emailed recovery code (SDK ergonomic shape). */
export interface RedeemEmailOpts {
  token: string;                // the code delivered to the member's inbox
  newPassword: string;          // replaces the credential on redeem
}

/**
 * Options for claiming a legacy account via an email-bound activation token
 * (AD-app-forums §8.4) — admits a member with no prior credential.
 */
export interface ClaimAccountOpts {
  email: string;                // the address the activation token is bound to
  token: string;                // the emailed activation token
  newPassword: string;          // the account's first password
}

/**
 * The personal-sign challenge a wallet/DID scheme signs before login — body of
 * GET /api/v1/login/challenge (webapi.ChallengeResponse; drift-guarded via the
 * login.json fixture).
 */
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
  /**
   * Request an email recovery code (always resolves whether or not the address
   * is known — no existence oracle).  Throws AmpError code 'Unsupported' when
   * the host has no email credential store — treat that as "hide the email
   * form, offer wallet login" (never dead-end the user).
   */
  recoverEmail: (email: string) => Promise<void>;
  /** Redeem an emailed recovery code — sets the new password and signs in. */
  redeemEmail: (opts: RedeemEmailOpts) => Promise<AmpMember>;
  /** Claim a legacy account via its emailed activation token — sets the first password and signs in. */
  claimAccount: (opts: ClaimAccountOpts) => Promise<AmpMember>;
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

/**
 * Access levels (amp.Access enum names, per the wire contract) — the full
 * vocabulary the grant endpoints accept (invite issue, governance grant).
 * 'NotAllowed' as a governance grant is an explicit deny.
 */
export type AccessLevel =
  | 'NotAllowed' | 'Invite' | 'Private'
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

// ── NameService / federation directory (wire shapes) ────────────────
//
// POST /api/v1/resolve (anonymous), POST /api/v1/search (Bearer),
// GET /api/v1/federation/peers (Bearer) — mirroring amp.SDK/amp/webapi
// (drift-guarded via the vault.json fixture).  See SKILL §4.6.

/**
 * The three-state back-edge verdict on a NameService record (amp.TrustState
 * enum names; DD-name-service §3.3).  Load-bearing: never silently follow a
 * non-Verified or Ambiguous answer — surface it and let the user choose.
 */
export type TrustState = 'Unchecked' | 'Verified' | 'Refuted';

/** Where a planet's vault is dialable.  Address is base64 transport bytes (Go []byte), NOT a base32 UID. */
export interface VaultEndpoint {
  Transport: string;     // "tcp", "udp", "reticulum", …
  Address: string;       // base64 transport-specific encoding
}

/** An exact-match FQDN → planet resolution (webapi.ResolveResponse). */
export interface ResolveResponse {
  FQDN: string;
  PlanetID: string;             // base32 tag.UID — the planet the FQDN names
  AnsweredBy: string;           // base32 tag.UID — the federation that answered
  VaultAddrs?: VaultEndpoint[]; // dialable bootstrap addrs — returned in full by resolve
  TrustState: TrustState;
  PinPrecedence: boolean;
  Ambiguous: boolean;           // >1 federation claims this FQDN
  Hops: number;                 // forwarding hops to the answer
}

/** One ranked search result (webapi.SearchMatch — mirrors nameservice.Match + Snippet). */
export interface SearchMatch {
  PlanetID: string;
  FQDN: string;
  AnsweredBy: string;
  Score: number;
  AppName: string;
  AppDesc: string;
  Platforms?: string[];
}

/** A peer / parent pointer a federation enumerates for cross-federation forwarding. */
export interface FederationPeerEntry {
  FederationID: string;         // base32 tag.UID
  VaultAddrs?: VaultEndpoint[];
  Label?: string;
}

// ── Brand — substrate planet identity (wire shapes) ─────────────────
//
// One Brand item per planet at the head-node anchor (std.Attr.Brand), read
// over the standard items rail.  DISPLAY-ONLY by rule (SKILL §10): admin-
// mutable, so never a source of app behavior.

/**
 * The amp.Tag JSON wire shape (every field omitempty Go-side).  BlobRef is
 * the media-specific view of the same wire Tag; this is the general form
 * (Brand.Identity.NamedBy, Brand.TemplateSet).
 */
export interface AmpTag {
  UID?: string;          // base32 tag.UID
  I?: number;
  J?: number;
  K?: number;
  Units?: number;
  ContentTypeRaw?: string;
  URI?: string;
  Text?: string;
}

/** A planet's identity field set (amp.BrandIdentity). */
export interface BrandIdentity {
  AppName?: string;      // public name of the planet ("Tunr")
  OrgName?: string;      // operator / sponsoring org ("SoundSpectrum")
  AppDomain?: string;    // canonical domain the planet presents as
  AppDesc?: string;
  URLSchemes?: string[]; // deep-link schemes the planet answers to
  NamedBy?: AmpTag;      // the federation naming this planet — the SKILL §4.6 back-edge
}

/** One per-platform install target (amp.AppTarget).  Platform is the PlatformID enum's wire integer. */
export interface AppTarget {
  Platform?: number;
  DownloadURL?: string;
  BundleID?: string;
  MinOSVersion?: string;
  AppleTeamID?: string;
}

/** One curated entry point (amp.AppLink). */
export interface AppLink {
  Label?: string;
  URL?: string;
}

/** A crate reference (amp.CrateRef JSON: BlobID rides as base32). */
export interface CrateRef {
  CrateURI?: string;
  BlobID?: string;
}

/** A planet's substrate Brand record (amp.Brand; admin-mutable → display-only). */
export interface Brand {
  Identity?: BrandIdentity;
  OrgHomeURL?: string;
  AppHomeURL?: string;
  Targets?: AppTarget[];
  CrateSnapshotURL?: string;
  Links?: AppLink[];
  BundledCrates?: CrateRef[];
  TemplateSet?: AmpTag;
}

/**
 * getBrand()'s composite result (SDK ergonomic shape).
 *
 * `trustState` is the host resolver's verdict on the Brand's CLAIMED
 * `Identity.AppDomain` record — 'Unchecked' when the Brand claims no domain
 * or no federation names it.  The verdict binds (FQDN → resolution.PlanetID),
 * not the planet you queried: when `resolution.PlanetID` differs from your
 * planet's UID, the domain claim points elsewhere — surface it, never render
 * a Verified badge (TrustState is load-bearing, SKILL §4.6).
 */
export interface PlanetBrand {
  brand: Brand;                  // the substrate Brand (display-only, SKILL §10)
  namedBy: string;               // base32 federation UID from Brand.Identity.NamedBy ('' = unset)
  trustState: TrustState;
  resolution?: ResolveResponse;  // present when the claimed AppDomain resolved
}

/** useAmpResolve() result.  resolution === null with no error = no federation names that FQDN. */
export interface AmpResolveResult {
  resolution: ResolveResponse | null;
  loading: boolean;
  error: Error | null;
}

/** useAmpBrand() result.  brand === null with no error = the planet carries no Brand (naked home planet). */
export interface AmpBrandResult {
  brand: PlanetBrand | null;
  loading: boolean;
  error: Error | null;
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
  | { type: 'error'; Channel?: string; Attr?: string; Error: string }
  // Client-synthesized (never a wire frame): the WebSocket dropped and has
  // re-opened.  Frames pushed during the outage are lost — there is no
  // server-side resume cursor — so subscribers must refetch to re-sync
  // (useAmpQuery does this automatically).
  | { type: 'reconnect' };
