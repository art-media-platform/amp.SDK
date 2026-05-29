/**
 * AmpAdapter is the interface the hooks call — never `fetch` directly.
 *
 * AmpWebClient is the one shipped implementation: it speaks the amp.exe
 * `app.www` wire contract (amp.SDK/amp/webapi).
 */

import type { KeyPair } from './crypto/types.js';
import type {
  AmpItemMeta,
  AmpMember,
  AmpQueryOpts,
  BlobRef,
  Address,
  LoginCredentials,
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
  getSession(): AmpMember | null;

  /** Subscribe to auth state changes; returns unsubscribe function. */
  onAuthChange(callback: (member: AmpMember | null) => void): () => void;

  /** Fetch the EIP-4361 (SIWE) challenge for `address` to sign before login(scheme:'wallet'). */
  getWalletChallenge(address: string): Promise<WalletChallenge>;

  // ── CRUD ──────────────────────────────────────────────────────────

  query<T>(
    channel: string,
    attr: string,
    opts?: AmpQueryOpts,
  ): Promise<{ data: (T & AmpItemMeta)[]; hasMore: boolean; next?: string }>;

  /** Canonical batched write — one TxMsg, N ops, one signature + MemberProof. */
  tx(ops: TxOp[], planetTag?: string): Promise<TxResult[]>;

  create(channel: string, attr: string, value: Record<string, unknown>): Promise<string>;
  upsert(channel: string, attr: string, itemID: string, value: Record<string, unknown>): Promise<void>;
  remove(channel: string, attr: string, itemID: string): Promise<void>;
  withdraw(channel: string, attr: string, itemID: string, opts: WithdrawOpts): Promise<void>;

  // ── Tag resolution (server canonization) ──────────────────────────

  resolveTag(expr: string): Promise<TagResolution>;
  resolveTags(exprs: string[]): Promise<TagResolution[]>;

  // ── Media ─────────────────────────────────────────────────────────

  upload(file: File, channel: string, opts?: UploadOpts): Promise<BlobRef>;

  /** Caller-carries-the-Tag resolve: BlobRef → BlobRef with URI (stream URL) set. */
  resolveMedia(blob: BlobRef): Promise<BlobRef>;

  /** Direct /www/{UID} URL for an already-published blob. */
  mediaUrl(blobUID: string): Promise<string>;

  // ── Addresses (cross-planet CRDT-cell references, DESIGN-12) ──────

  /** Build an Address for embedding in shares / withdraw delegations. */
  address(ref: Address): Address;

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
}
