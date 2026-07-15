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
  InviteAcceptOpts,
  InviteAcceptResult,
  InviteIssueOpts,
  InviteIssueResult,
  InviteListResult,
  InviteRevokeOpts,
  LoginCredentials,
  RedeemEmailOpts,
  SubscriptionEvent,
  TagResolution,
  TxOp,
  TxResult,
  UploadOpts,
  WalletChallenge,
  WithdrawOpts,
} from './types.js';

export interface AmpAdapter {
  // ── Auth ──────────────────────────────────────────────────────────

  login(credentials: LoginCredentials): Promise<AmpMember>;
  logout(): Promise<void>;

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

  // ── Media ─────────────────────────────────────────────────────────

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

  /** Seal plaintext to the session member (anonymous-sender HPKE base mode). */
  seal(plaintext: Uint8Array): Promise<Uint8Array>;

  /** Open sealed bytes with the session member's EncryptKey. */
  open(sealed: Uint8Array): Promise<Uint8Array>;

  /** The installed EncryptKey's public ref, or null when BYOK isn't installed. */
  getEncryptPub(): PubKeyRef | null;
}
