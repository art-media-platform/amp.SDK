/**
 * AmpAdapter is the interface the hooks call — never `fetch` directly.
 *
 * AmpWebClient is the one shipped implementation: it speaks the `ampd`
 * `app.www` wire contract (amp.SDK/amp/webapi).
 */

import type { KeyPair, PubKeyRef } from './crypto/types.js';
import type {
  AmpItemMeta,
  AmpMember,
  AmpQueryOpts,
  AmpSession,
  BlobRef,
  ClaimAccountOpts,
  FederationPeerEntry,
  InviteAcceptOpts,
  InviteAcceptResult,
  InviteIssueOpts,
  InviteIssueResult,
  InviteListResult,
  InviteRevokeOpts,
  LoginCredentials,
  PlanetBrand,
  RedeemEmailOpts,
  ResolveResponse,
  SearchMatch,
  SessionRevokeResult,
  SubscriptionEvent,
  TagResolution,
  TxOp,
  TxResult,
  UploadOpts,
  WalletChallenge,
  WithdrawOpts,
} from './types.js';

export interface AmpAdapter {
  /**
   * The adapter's default planet tag ('' / absent = the session's bound
   * planet).  Hooks compare a per-call planetTag against this to detect a
   * cross-planet read, whose rows must not merge session-planet WS events
   * (subscriptions always bind the session planet — SKILL §4.5).
   */
  readonly defaultPlanetTag?: string;

  // ── Auth ──────────────────────────────────────────────────────────

  login(credentials: LoginCredentials): Promise<AmpMember>;
  logout(): Promise<void>;

  /**
   * Member self-revoke ("sign out everywhere") — POST /api/v1/session/revoke.
   * FULL revoke including the calling session; the local session drops on
   * success and a fresh login is required afterward.
   */
  sessionRevoke(): Promise<SessionRevokeResult>;

  /** The locally-held member (sync, no I/O) — null on a fresh load until restoreSession(). */
  getSession(): AmpMember | null;

  /**
   * Rehydrate a persisted session on a fresh load, re-validated against the
   * host; resolves the member, or null when signed out (AmpProvider calls
   * this on mount so a reload lands authenticated).
   */
  restoreSession(): Promise<AmpMember | null>;

  /** GET /api/v1/session — host-validated session state.  AmpError(401) when none is bound. */
  fetchSession(): Promise<AmpSession>;

  /** GET /api/v1/me — the authenticated member's record.  AmpError(401) when unauthenticated. */
  me(): Promise<AmpMember>;

  /** Subscribe to auth state changes; returns unsubscribe function. */
  onAuthChange(callback: (member: AmpMember | null) => void): () => void;

  /** Fetch the EIP-4361 (SIWE) challenge for `address` to sign before login(scheme:'wallet'). */
  getWalletChallenge(address: string): Promise<WalletChallenge>;

  /** Fetch the challenge bound to a DID URI to sign before login(scheme:'did'). */
  getDIDChallenge(did: string): Promise<WalletChallenge>;

  /**
   * Request an email recovery code — resolves on the uniform 202 whether or
   * not the address is known (no existence oracle).  AmpError('Unsupported')
   * when the host has no email credential store: fall back to wallet login.
   */
  recoverEmail(email: string): Promise<void>;

  /** Redeem an emailed recovery code — sets the new password and mints the session (doubles as login). */
  redeemEmail(opts: RedeemEmailOpts): Promise<AmpMember>;

  /** Claim a legacy account via its emailed activation token (AD-app-forums §8.4) — first password + session. */
  claimAccount(opts: ClaimAccountOpts): Promise<AmpMember>;

  // ── CRUD ──────────────────────────────────────────────────────────

  query<T>(
    channel: string,
    attr: string,
    opts?: AmpQueryOpts,
  ): Promise<{ data: (T & AmpItemMeta)[]; hasMore: boolean; next?: string }>;

  /** Canonical batched write — one TxMsg, N ops, one signature + MemberProof. */
  tx(ops: TxOp[], planetTag?: string): Promise<TxResult[]>;

  /**
   * Invoke an app verb: route the ops to the named verb URL's handler instead of
   * the cabinet, carrying the session member as the authoring caller.  The app
   * reads the ops as RPC arguments and authors any durable writes itself
   * (custodially).  Form: "amp://~/{app}/{verb}" (e.g. "amp://~/forums/post").
   */
  invoke(verbURL: string, ops: TxOp[], planetTag?: string): Promise<TxResult[]>;

  create(channel: string, attr: string, value: Record<string, unknown>): Promise<string>;
  upsert(channel: string, attr: string, itemID: string, value: Record<string, unknown>): Promise<void>;
  remove(channel: string, attr: string, itemID: string): Promise<void>;
  withdraw(channel: string, attr: string, itemID: string, opts: WithdrawOpts): Promise<void>;

  // ── Tag resolution (server canonization) ──────────────────────────

  resolveTag(expr: string): Promise<TagResolution>;
  resolveTags(exprs: string[]): Promise<TagResolution[]>;

  // ── NameService / federation directory (SKILL §4.6) ───────────────

  /** Resolve a registered FQDN to its planet (anonymous; AmpError 404 = no record).  Never silently follow a non-Verified TrustState. */
  resolve(fqdn: string): Promise<ResolveResponse>;

  /** Ranked search over the session's joined federations (Bearer; membership-gated enumeration). */
  search(query: string, limit?: number): Promise<SearchMatch[]>;

  /** A federation's peer / parent pointers for cross-federation forwarding (Bearer; UID required). */
  federationPeers(federationID: string): Promise<FederationPeerEntry[]>;

  // ── Brand (read-only substrate identity — SKILL §10) ──────────────

  /** A planet's substrate Brand + NamedBy + the resolver's TrustState verdict on its claimed AppDomain; null = no Brand authored.  Display-only. */
  getBrand(planetTag?: string): Promise<PlanetBrand | null>;

  // ── Media ─────────────────────────────────────────────────────────

  /** Store a blob; a file larger than one chunk rides the sequential chunk door (UploadOpts.chunked / chunkSize).  Progress ticks per chunk ack. */
  upload(file: File, channel: string, opts?: UploadOpts): Promise<BlobRef>;

  /** Caller-carries-the-Tag resolve: BlobRef → BlobRef with URI (stream URL) set.
   *  Pass planetTag to resolve a blob on another planet (e.g. an anonymous public share). */
  resolveMedia(blob: BlobRef, planetTag?: string): Promise<BlobRef>;

  /** Direct /www/{UID} URL for an already-published blob (pure string build, no I/O). */
  mediaUrl(blobUID: string): string;

  // ── Federation invites ────────────────────────────────────────────
  // Member-session tier (planet-admin Bearer for issue/revoke/list) — NOT the
  // operator tier, which deliberately has no client binding (SKILL §12).

  /** Mint a sealed invite on a planet the session administers (SKILL §4.7). */
  issueInvite(opts: InviteIssueOpts): Promise<InviteIssueResult>;

  /** Redeem a sealed invite (universal URL or bare amp-base32 body) to join its federation planet (Bearer; see SKILL §4.7). */
  acceptInvite(opts: InviteAcceptOpts): Promise<InviteAcceptResult>;

  /** Terminally revoke an invite policy (reissue rather than re-arm). */
  revokeInvite(opts: InviteRevokeOpts): Promise<void>;

  /** A planet's invite policies with their rank-adjudicated redemption state. */
  listInvites(planet: string): Promise<InviteListResult>;

  // ── Subscriptions ─────────────────────────────────────────────────

  /** Subscribe to live changes on a channel+attr; returns unsubscribe function. */
  subscribe(
    channel: string,
    attr: string,
    callback: (event: SubscriptionEvent) => void,
  ): () => void;

  // ── Sealed-box BYOK ───────────────────────────────────────────────

  /**
   * Override the auto-installed device EncryptKey (login installs one), or
   * pass null to clear.  seal/open work after login without calling this.
   */
  setEncryptKey(keyPair: KeyPair | null): void;

  /** Seal plaintext to the session member as a sealed box (anonymous-sender). */
  seal(plaintext: Uint8Array): Promise<Uint8Array>;

  /** Open sealed bytes with the session member's EncryptKey. */
  open(sealed: Uint8Array): Promise<Uint8Array>;

  /** The installed EncryptKey's public ref, or null when BYOK isn't installed. */
  getEncryptPub(): PubKeyRef | null;
}
