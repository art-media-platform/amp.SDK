/**
 * window.amp — the WebRect card bridge (SKILL §8; AD-app-www.md §6.5).
 *
 * A card is a self-contained HTML document rendered inside a Unity WebRect Pane
 * (3D / XR) or a 2D browser drawer.  The node serves the implementation as the
 * card's first script (`/amp/bridge.js`); it installs `window.amp` at once and
 * attaches to the host's transport hook when one arrives.  A card codes against
 * the bridge instead of importing the SDK client.  These are the card-author
 * types — the served script supplies the implementation — so a card gets
 * autocomplete and type-checking for `window.amp.*`.
 */

import type { Address, TxOp, WithdrawOpts, BlobRef, UploadOpts, SubscriptionEvent } from './types.js';

/** Options for AmpBridge.list — a card-scoped read window over a channel+attr. */
export interface ListOpts {
  limit?: number;
  after?: string;                     // cursor (itemID to start after)
  filter?: Record<string, unknown>;   // client-side equality filter hint
}

/** Receipt for a batched bridge tx: "transmission received; queued for delivery + processing". */
export interface TxReceipt {
  txID: string;
  accepted: boolean;
}

/** A form submission a card hands to the host (AmpBridge.submit). */
export interface FormPayload {
  intent: string;
  values: Record<string, unknown>;
  channel?: string;
  attr?: string;
  itemID?: string;
}

/** Outcome of AmpBridge.submit. */
export interface SubmitResult {
  ok: boolean;
  itemID?: string;
  error?: string;
}

/** The card's subject — the cell the card was opened on, read off its URL query. */
export interface BridgeItem {
  PlanetID: string;
  ChannelID: string;
  ItemID: string;
}

/** The card's view of its session member (a subset of AmpIdentity). */
export interface BridgeMember {
  ID: string;
  DisplayName: string;
  PlanetID: string;
  Kind?: string;
}

/**
 * The `window.amp` surface a card calls.  Data verbs mirror the SDK client; the
 * navigation / form / live-var verbs are the Card ↔ Pane channel.
 */
export interface AmpBridge {
  // ── Identity ──
  member: BridgeMember | null;                     // null until the host attaches
  item: BridgeItem;                                // the card's subject, from the card URL's ?planet= &channel= &item= ('' when absent)
  theme: Record<string, string>;                   // the host's CSS custom properties, set on :root at attach; {} until then
  onReady(cb: (amp: AmpBridge) => void): void;     // the host attached; member is set (fires at once when already attached)

  // ── Data ──
  read(channel: string, attr: string, itemID: string): Promise<unknown>;
  list(channel: string, attr: string, opts?: ListOpts): Promise<unknown[]>;
  tx(ops: TxOp[]): Promise<TxReceipt>;                                                   // batched write — one TxMsg, N ops
  write(channel: string, attr: string, itemID: string, value: unknown): Promise<void>;  // sugar: tx with one upsert
  remove(channel: string, attr: string, itemID: string): Promise<void>;                 // sugar: tx with one remove
  withdraw(channel: string, attr: string, itemID: string, opts: WithdrawOpts): Promise<void>;
  subscribe(channel: string, attr: string, cb: (event: SubscriptionEvent) => void): () => void;

  // ── Media ──
  upload(channel: string, opts?: UploadOpts): Promise<BlobRef>;
  resolveMedia(blob: BlobRef, planetTag?: string, cell?: Address): Promise<BlobRef>;  // returns blob with URI (stream URL) set; cell, else blob.Cell, names the BlobRef cell a sealed blob resolves by

  // ── Sealed secrets ──
  seal(plaintext: Uint8Array): Promise<Uint8Array>;
  open(sealed: Uint8Array): Promise<Uint8Array>;

  // ── Card navigation ──
  close(): void;                              // the host closes the card (its Pane or drawer)
  navigate(cardUrl: string): void;            // push next card on the stack
  back(): void;                               // pop current card
  setTitle(title: string): void;              // card tells Pane its title
  focus(elementID: string): void;             // request keyboard focus
  onBack(cb: () => boolean): void;            // intercept back; return true to consume
  onFocusChanged(cb: (hasFocus: boolean) => void): void;
  onScroll(cb: (delta: number) => void): void;  // iPod-wheel input

  // ── Form submission ──
  submit(form: FormPayload): Promise<SubmitResult>;

  // ── Live vars (Card ↔ Pane) ──
  setVar(key: string, value: unknown): void;
  onVar(key: string, cb: (value: unknown) => void): void;
}

declare global {
  interface Window {
    amp?: AmpBridge;
  }
}
