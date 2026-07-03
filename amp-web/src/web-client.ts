/**
 * AmpWebClient — talks the `ampd` `app.www` wire contract.
 *
 * REST:      {vaultUrl}/api/v1/...           (amp.SDK/amp/webapi)
 * WebSocket: {vaultUrl}/ws                   (flat SubscribeFrame fan-out)
 * Media:     {vaultUrl}/www/{UID}
 * Auth:      Authorization: Bearer {sessionToken}
 *
 * Wire JSON keys are PascalCase and UIDs are base32 strings — the SDK passes
 * UID strings straight through (no fixed64 codec).  Native fetch + WebSocket,
 * no external dependencies.
 */

import type { AmpAdapter } from './adapter.js';
import { createAmpCrypto } from './crypto/index.js';
import { AmpError, AmpErrorCode, ampErrorFromResponse } from './errors.js';
import { EmbedBridge } from './embed-bridge.js';
import {
  type EncryptKeyStorage,
  defaultEncryptKeyStorage,
  resolveDeviceEncryptKey,
} from './crypto/keystore.js';
import type { AmpCrypto, KeyPair, PubKeyRef } from './crypto/types.js';
import type {
  AmpItemMeta,
  AmpMember,
  AmpQueryOpts,
  BlobRef,
  InviteIssueOpts,
  InviteIssueResult,
  InviteAcceptOpts,
  InviteAcceptResult,
  InviteRevokeOpts,
  InviteListResult,
  LoginCredentials,
  SubscriptionEvent,
  TagResolution,
  TxOp,
  TxResult,
  UploadOpts,
  WalletChallenge,
  WithdrawNote,
  WithdrawOpts,
  WithdrawReason,
} from './types.js';

export interface AmpWebClientOpts {
  vaultUrl: string;       // operated node URL — e.g. https://prod.plan.tools
  planetTag: string;      // the planet this client reads/writes by default

  /**
   * Where the member's device-local EncryptKey is held for BYOK seal/open.
   * Defaults to IndexedDB in the browser and an in-memory store elsewhere.
   * Inject a custom store (e.g. an OS keychain bridge) to override.
   */
  encryptKeyStorage?: EncryptKeyStorage;
}

/** The wire Item shape returned by list/read endpoints (webapi.Item). */
interface WireItem {
  _ItemID: string;
  _EditID: string;
  _FromID: string;
  _UpdatedAt: string;
  Value: Record<string, unknown>;
  _Withdrawn?: WireWithdrawNote;
}

export class AmpWebClient implements AmpAdapter {
  private vaultUrl: string;
  private planetTag: string;
  private sessionToken: string | null = null;
  private member: AmpMember | null = null;
  private ws: WebSocket | null = null;
  private wsSubscriptions = new Map<string, Set<(event: SubscriptionEvent) => void>>();
  private authListeners: Set<(member: AmpMember | null) => void> = new Set();
  private wsReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private crypto: AmpCrypto = createAmpCrypto();
  private keyStorage: EncryptKeyStorage;
  private embed: EmbedBridge | null = null;

  constructor(opts: AmpWebClientOpts) {
    this.vaultUrl = opts.vaultUrl.replace(/\/$/, '');
    this.planetTag = opts.planetTag;
    this.keyStorage = opts.encryptKeyStorage ?? defaultEncryptKeyStorage();
    // Pick up the host-injected embed context: in the Unity host, window.__amp advertises the
    // verbs it handles natively and carries the SSO memberToken (AD-app-forums.md §6.4).
    this.embed = (typeof window !== 'undefined' && window.__amp?.embed) ? new EmbedBridge(window.__amp) : null;
  }

  // ── Internal helpers ─────────────────────────────────────────────

  private apiUrl(path: string): string {
    return `${this.vaultUrl}/api/v1${path}`;
  }

  private headers(): Record<string, string> {
    const hdrs: Record<string, string> = { 'Content-Type': 'application/json' };
    if (this.sessionToken) {
      hdrs['Authorization'] = `Bearer ${this.sessionToken}`;
    }
    return hdrs;
  }

  private async apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
    const resp = await fetch(this.apiUrl(path), {
      ...init,
      headers: { ...this.headers(), ...(init?.headers as Record<string, string> ?? {}) },
    });
    if (!resp.ok) {
      throw await ampErrorFromResponse(resp);
    }
    if (resp.status === 204) return undefined as T;
    return resp.json();
  }

  /** Encode a channel/attr/itemID path segment (canonic names + UIDs are slash-free). */
  private seg(s: string): string {
    return encodeURIComponent(s);
  }

  private itemsPath(channel: string, attr: string, itemID?: string, suffix?: string): string {
    let path = `/channels/${this.seg(channel)}/attrs/${this.seg(attr)}/items`;
    if (itemID) path += `/${this.seg(itemID)}`;
    if (suffix) path += `/${suffix}`;
    return path;
  }

  private planetQuery(planetTag?: string): string {
    const tag = planetTag ?? '';
    return tag ? `?planetTag=${encodeURIComponent(tag)}` : '';
  }

  private mapItem<T>(item: WireItem): T & AmpItemMeta {
    return {
      ...(item.Value as Record<string, unknown>),
      _ItemID: item._ItemID,
      _EditID: item._EditID,
      _FromID: item._FromID,
      _UpdatedAt: item._UpdatedAt,
      _Withdrawn: item._Withdrawn ? wireToWithdrawNote(item._Withdrawn) : undefined,
    } as T & AmpItemMeta;
  }

  // ── Auth ──────────────────────────────────────────────────────────

  async getWalletChallenge(address: string): Promise<WalletChallenge> {
    const resp = await fetch(`${this.vaultUrl}/api/v1/login/challenge?address=${encodeURIComponent(address)}`);
    if (!resp.ok) throw await ampErrorFromResponse(resp);
    return resp.json();
  }

  // getDIDChallenge fetches a challenge bound to a DID URI (did:key / did:pkh)
  // for the Scheme="did" login path.  Shares the single-use nonce store with the
  // wallet challenge; a did:pkh:eip155 DID gets the SIWE message, did:key a
  // generic domain-bound one.
  async getDIDChallenge(did: string): Promise<WalletChallenge> {
    const resp = await fetch(`${this.vaultUrl}/api/v1/login/challenge?did=${encodeURIComponent(did)}`);
    if (!resp.ok) throw await ampErrorFromResponse(resp);
    return resp.json();
  }

  async login(credentials: LoginCredentials): Promise<AmpMember> {
    const data = await this.apiFetch<{
      SessionToken: string;
      ExpiresAt: number;
      Member: AmpMember;
    }>('/login', {
      method: 'POST',
      body: JSON.stringify(credentials),
    });

    this.sessionToken = data.SessionToken;
    this.member = {
      ...data.Member,
      PlanetID: data.Member.PlanetID || this.planetTag,
    };

    // Install the member's device-local EncryptKey so seal/open work without
    // an out-of-band setEncryptKey call.  Best-effort: a storage failure leaves
    // BYOK uninstalled (seal/open then throw a clear "no EncryptKey" error)
    // rather than failing the login itself.
    await this.installDeviceEncryptKey(this.member.ID);

    this.authListeners.forEach(cb => cb(this.member));
    this.connectWs();
    return this.member;
  }

  /**
   * installDeviceEncryptKey resolves (or generates) the member's device-local
   * EncryptKey and installs it on the crypto surface.  A caller that wants to
   * supply its own key (e.g. derived from a wallet) can override afterwards via
   * setEncryptKey.
   */
  private async installDeviceEncryptKey(memberID: string): Promise<void> {
    if (!memberID) {
      return;
    }
    try {
      const keyPair = await resolveDeviceEncryptKey(this.keyStorage, memberID);
      this.crypto.setEncryptKey(keyPair);
    } catch {
      // Leave BYOK uninstalled — seal/open surface the actionable error.
    }
  }

  async logout(): Promise<void> {
    // Clear local secrets FIRST — a slow/hung /logout must not leave the Bearer
    // and the in-memory device key resident for the round-trip.
    const token = this.sessionToken;
    this.sessionToken = null;
    this.member = null;
    this.crypto.setEncryptKey(null);
    this.disconnectWs();
    this.authListeners.forEach(cb => cb(null));
    if (token) {
      await fetch(this.apiUrl('/logout'), {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {});
    }
  }

  getSession(): AmpMember | null {
    return this.member;
  }

  onAuthChange(callback: (member: AmpMember | null) => void): () => void {
    this.authListeners.add(callback);
    return () => { this.authListeners.delete(callback); };
  }

  // ── CRUD ──────────────────────────────────────────────────────────

  async query<T>(
    channel: string,
    attr: string,
    opts?: AmpQueryOpts,
  ): Promise<{ data: (T & AmpItemMeta)[]; hasMore: boolean; next?: string }> {
    // Single-item read.
    if (opts?.itemID) {
      const item = await this.apiFetch<WireItem>(
        this.itemsPath(channel, attr, opts.itemID) + this.planetQuery(opts.planetTag),
      );
      return { data: item ? [this.mapItem<T>(item)] : [], hasMore: false };
    }

    // List read.
    const params = new URLSearchParams();
    if (opts?.limit) params.set('limit', String(opts.limit));
    if (opts?.after) params.set('after', opts.after);
    if (opts?.planetTag) params.set('planetTag', opts.planetTag);
    const qs = params.toString();
    const out = await this.apiFetch<{ Items: WireItem[]; HasMore: boolean; Next?: string }>(
      this.itemsPath(channel, attr) + (qs ? `?${qs}` : ''),
    );
    return {
      data: (out.Items ?? []).map(item => this.mapItem<T>(item)),
      hasMore: out.HasMore,
      next: out.Next,
    };
  }

  async tx(ops: TxOp[], planetTag?: string): Promise<TxResult[]> {
    const wireOps = ops.map(op =>
      op.Withdraw ? { ...op, Withdraw: withdrawNoteToWire(op.Withdraw) } : op,
    );
    const out = await this.apiFetch<{ TxID: string; Results: TxResult[] }>('/tx', {
      method: 'POST',
      body: JSON.stringify({ Ops: wireOps, PlanetTag: planetTag }),
    });
    return out.Results ?? [];
  }

  async invoke(verbURL: string, ops: TxOp[], planetTag?: string): Promise<TxResult[]> {
    // Embedded in the host + the host handles this verb natively → divert so the member's OWN
    // key signs the write (e.g. a self-signed forums reply); reads + other writes stay custodial.
    if (this.embed?.routes(verbURL)) {
      if (ops.length !== 1) {
        throw new Error(`embedded divert of ${verbURL} expects exactly one op, got ${ops.length}`);
      }
      const op = ops[0];
      return this.embed.invoke(verbURL, op.Value, planetTag ?? this.planetTag, op.Channel);
    }
    const wireOps = ops.map(op =>
      op.Withdraw ? { ...op, Withdraw: withdrawNoteToWire(op.Withdraw) } : op,
    );
    const out = await this.apiFetch<{ TxID: string; Results: TxResult[] }>('/tx', {
      method: 'POST',
      body: JSON.stringify({ Ops: wireOps, PlanetTag: planetTag, InvokeURL: verbURL }),
    });
    return out.Results ?? [];
  }

  async create(channel: string, attr: string, value: Record<string, unknown>): Promise<string> {
    const out = await this.apiFetch<{ Results: TxResult[] }>(
      this.itemsPath(channel, attr),
      { method: 'POST', body: JSON.stringify(value) },
    );
    return out.Results?.[0]?.ItemID ?? '';
  }

  async upsert(channel: string, attr: string, itemID: string, value: Record<string, unknown>): Promise<void> {
    await this.apiFetch(this.itemsPath(channel, attr, itemID), {
      method: 'PUT',
      body: JSON.stringify(value),
    });
  }

  async remove(channel: string, attr: string, itemID: string): Promise<void> {
    await this.apiFetch(this.itemsPath(channel, attr, itemID), { method: 'DELETE' });
  }

  async withdraw(channel: string, attr: string, itemID: string, opts: WithdrawOpts): Promise<void> {
    const body = withdrawNoteToWire({
      Reason: opts.reason,
      Rationale: opts.rationale,
      Subject: opts.subject,
      Delegation: opts.delegation,
    });
    await this.apiFetch(this.itemsPath(channel, attr, itemID, 'withdraw'), {
      method: 'POST',
      body: JSON.stringify(body),
    });
  }

  // ── Tag resolution ────────────────────────────────────────────────

  async resolveTag(expr: string): Promise<TagResolution> {
    return this.apiFetch<TagResolution>(`/tag/resolve?expr=${encodeURIComponent(expr)}`);
  }

  async resolveTags(exprs: string[]): Promise<TagResolution[]> {
    const out = await this.apiFetch<{ Results: TagResolution[] }>('/tag/resolve', {
      method: 'POST',
      body: JSON.stringify({ Exprs: exprs }),
    });
    return out.Results ?? [];
  }

  // ── Media ─────────────────────────────────────────────────────────

  async upload(file: File, channel: string, opts?: UploadOpts): Promise<BlobRef> {
    const form = new FormData();
    form.append('file', file);
    form.append('channel', channel);
    if (opts?.attr) form.append('attr', opts.attr);
    if (opts?.planetTag) form.append('planetTag', opts.planetTag);
    if (opts?.metadata) form.append('metadata', JSON.stringify(opts.metadata));

    const hdrs: Record<string, string> = {};
    if (this.sessionToken) {
      hdrs['Authorization'] = `Bearer ${this.sessionToken}`;
    }
    // Don't set Content-Type — the browser sets the multipart boundary.

    const resp = await fetch(this.apiUrl('/upload'), { method: 'POST', headers: hdrs, body: form });
    if (!resp.ok) {
      throw await ampErrorFromResponse(resp);
    }
    opts?.onProgress?.(100);
    return resp.json();
  }

  async resolveMedia(blob: BlobRef, planetTag?: string): Promise<BlobRef> {
    return this.apiFetch<BlobRef>('/media/resolve', {
      method: 'POST',
      body: JSON.stringify({ Blob: blob, PlanetTag: planetTag }),
    });
  }

  mediaUrl(blobUID: string): string {
    return `${this.vaultUrl}/www/${encodeURIComponent(blobUID)}`;
  }

  // ── Invites ───────────────────────────────────────────────────────

  // issueInvite mints an invite on a planet the session administers.  Omit
  // maxRedemptions (or 0) for a single-use pre-minted slot; set it for a
  // multi-use self-mint policy.  Returns the invite ID + its universal-URL text
  // (deliver the URL and the passphrase over separate channels).  Bearer.
  async issueInvite(opts: InviteIssueOpts): Promise<InviteIssueResult> {
    return this.apiFetch<InviteIssueResult>('/invite/issue', {
      method: 'POST',
      body: JSON.stringify({
        Planet: opts.planet,
        Passphrase: opts.passphrase,
        MaxRedemptions: opts.maxRedemptions ?? 0,
        Access: opts.access ?? '',
        ExpiresAt: opts.expiresAt ?? 0,
        VaultAddrs: opts.vaultAddrs ?? [],
      }),
    });
  }

  // acceptInvite redeems a sealed invite (its universal URL or amp-base32 body)
  // to join the planet, minting this member's keys host-side.  Bearer; the
  // passphrase arrives out-of-band and the token is inert without it.
  async acceptInvite(opts: InviteAcceptOpts): Promise<InviteAcceptResult> {
    return this.apiFetch<InviteAcceptResult>('/invite/accept', {
      method: 'POST',
      body: JSON.stringify({ InviteText: opts.inviteText, Passphrase: opts.passphrase }),
    });
  }

  // revokeInvite terminally revokes an invite's policy (reissue rather than
  // un-revoke).  Set rotate to also rotate the planet epoch, retiring the
  // token-held key (node-custodial founder only).  Bearer.
  async revokeInvite(opts: InviteRevokeOpts): Promise<void> {
    await this.apiFetch<InviteListResult>('/invite/revoke', {
      method: 'POST',
      body: JSON.stringify({
        Planet: opts.planet,
        InviteID: opts.inviteId ?? '',
        InviteText: opts.inviteText ?? '',
        Rotate: opts.rotate ?? false,
      }),
    });
  }

  // listInvites returns a planet's invite policies with their rank-adjudicated
  // redemption ledgers.  Bearer.
  async listInvites(planet: string): Promise<InviteListResult> {
    return this.apiFetch<InviteListResult>(
      `/invite/list?planet=${encodeURIComponent(planet)}`,
      { method: 'GET' },
    );
  }

  // ── WebSocket subscriptions ───────────────────────────────────────

  subscribe(
    channel: string,
    attr: string,
    callback: (event: SubscriptionEvent) => void,
  ): () => void {
    const key = `${channel}:${attr}`;
    let subs = this.wsSubscriptions.get(key);
    if (!subs) {
      subs = new Set();
      this.wsSubscriptions.set(key, subs);
    }
    subs.add(callback);
    this.wsSend({ Type: 'subscribe', Channel: channel, Attr: attr });

    return () => {
      subs!.delete(callback);
      if (subs!.size === 0) {
        this.wsSubscriptions.delete(key);
        this.wsSend({ Type: 'unsubscribe', Channel: channel, Attr: attr });
      }
    };
  }

  // ── WebSocket internals ───────────────────────────────────────────

  private connectWs(): void {
    if (this.ws) return;
    // No WebSocket in SSR / non-browser hosts — REST stays usable; live
    // subscriptions resume on a client that has one.
    if (typeof WebSocket === 'undefined') return;

    const secure = this.vaultUrl.startsWith('https');
    const host = this.vaultUrl.replace(/^https?:\/\//, '');
    // The Bearer rides the WS URL query (browsers can't set headers on the
    // upgrade), so a cleartext ws:// would leak it on the wire and into proxy
    // logs.  Refuse unless the host is loopback (local dev over http is fine).
    const isLocal = /^(localhost|127\.0\.0\.1|\[::1\])(:|$)/.test(host);
    if (!secure && !isLocal) {
      throw new AmpError(
        0,
        AmpErrorCode.BadRequest,
        `refusing insecure WebSocket to ${host}: the session token would travel in cleartext — use https:// (wss://)`,
      );
    }
    const protocol = secure ? 'wss' : 'ws';
    const url = `${protocol}://${host}/ws?token=${encodeURIComponent(this.sessionToken ?? '')}`;

    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      for (const key of this.wsSubscriptions.keys()) {
        const [channel, attr] = key.split(':');
        this.wsSend({ Type: 'subscribe', Channel: channel, Attr: attr });
      }
    };

    this.ws.onmessage = (evt) => {
      let frame: WireSubscribeFrame;
      try {
        frame = JSON.parse(evt.data as string);
      } catch {
        return;
      }
      const event = frameToEvent(frame);
      if (!event) return;
      const subs = this.wsSubscriptions.get(`${frame.Channel}:${frame.Attr}`);
      if (subs) subs.forEach(cb => cb(event));
    };

    this.ws.onclose = () => {
      this.ws = null;
      if (this.sessionToken) {
        this.wsReconnectTimer = setTimeout(() => this.connectWs(), 3000);
      }
    };

    this.ws.onerror = () => {
      this.ws?.close();
    };
  }

  private disconnectWs(): void {
    if (this.wsReconnectTimer) {
      clearTimeout(this.wsReconnectTimer);
      this.wsReconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  private wsSend(msg: Record<string, unknown>): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  // ── Sealed-box BYOK ───────────────────────────────────────────────

  // login() auto-installs the member's device-local EncryptKey, so seal/open
  // work without calling this.  Use it only to override with a key sourced
  // elsewhere (e.g. derived from a wallet), or pass null to clear.
  setEncryptKey(keyPair: KeyPair | null): void {
    this.crypto.setEncryptKey(keyPair);
  }

  seal(plaintext: Uint8Array): Promise<Uint8Array> {
    return this.crypto.seal(plaintext);
  }

  open(sealed: Uint8Array): Promise<Uint8Array> {
    return this.crypto.open(sealed);
  }

  // The installed EncryptKey's public ref, or null when BYOK isn't installed
  // (seal/open would then throw).  Gate a must-be-sealed write on this rather
  // than catching seal()'s throw and silently falling back to plaintext.
  getEncryptPub(): PubKeyRef | null {
    return this.crypto.getEncryptPub();
  }
}

/** Wire-shape WithdrawNote (PascalCase; base32 UID strings). */
interface WireWithdrawNote {
  Reason?: WithdrawReason;
  Rationale?: string;
  WithdrawnAt?: string;
  WithdrawnBy?: string;  // base32 member UID
  Subject?: string;      // base32 member UID
  Delegation?: string;   // amp.Address packed base32
}

/** Wire-shape SubscribeFrame (PascalCase). */
interface WireSubscribeFrame {
  Type: string;
  Channel?: string;
  Attr?: string;
  ItemID?: string;
  EditID?: string;
  FromID?: string;
  Value?: Record<string, unknown>;
  UpdatedAt?: string;
  Withdraw?: WireWithdrawNote;
  Error?: string;
}

/** Decode a flat webapi.SubscribeFrame into a typed SubscriptionEvent. */
function frameToEvent(frame: WireSubscribeFrame): SubscriptionEvent | null {
  // An error frame carries no ItemID — surface it before the data-frame guard so
  // a rejected subscribe reaches its (channel, attr) subscribers.
  if (frame.Type === 'error') {
    return { type: 'error', Channel: frame.Channel, Attr: frame.Attr, Error: frame.Error ?? 'subscription error' };
  }
  if (!frame.ItemID) return null;
  switch (frame.Type) {
    case 'update':
      return {
        type: 'update',
        ItemID: frame.ItemID,
        Value: frame.Value ?? {},
        EditID: frame.EditID ?? '',
        FromID: frame.FromID ?? '',
        UpdatedAt: frame.UpdatedAt,
      };
    case 'delete':
      return { type: 'delete', ItemID: frame.ItemID, EditID: frame.EditID, FromID: frame.FromID };
    case 'withdraw':
      return {
        type: 'withdraw',
        ItemID: frame.ItemID,
        EditID: frame.EditID,
        FromID: frame.FromID,
        Withdraw: wireToWithdrawNote(frame.Withdraw),
      };
    default:
      return null;
  }
}

/** Translate a wire WithdrawNote into the SDK shape (base32 UID strings). */
function wireToWithdrawNote(w: WireWithdrawNote | undefined): WithdrawNote {
  return {
    // The server always stamps a Reason on a stored withdraw, so this fallback is
    // unreachable in practice; it only satisfies the required field on decode.
    Reason: w?.Reason ?? 'Retracted',
    Rationale: w?.Rationale,
    WithdrawnAt: w?.WithdrawnAt,
    WithdrawnBy: w?.WithdrawnBy || undefined,
    Subject: w?.Subject || undefined,
    Delegation: w?.Delegation || undefined,
  };
}

/** Translate an SDK WithdrawNote into the wire shape. */
function withdrawNoteToWire(note: WithdrawNote): Record<string, unknown> {
  const out: Record<string, unknown> = { Reason: note.Reason };
  if (note.Rationale) out.Rationale = note.Rationale;
  if (note.Subject) out.Subject = note.Subject;
  if (note.Delegation) out.Delegation = note.Delegation;
  return out;
}
