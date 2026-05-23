/**
 * AmpWebClient — talks the amp.exe `app.www` wire contract.
 *
 * REST:      {vaultUrl}/api/v1/...           (amp.SDK/amp/webapi)
 * WebSocket: {vaultUrl}/ws                   (flat SubscribeFrame fan-out)
 * Media:     {vaultUrl}/www/{id}
 * Auth:      Authorization: Bearer {sessionToken}
 *
 * Native fetch + WebSocket — no external dependencies.
 */

import type { AmpAdapter } from './adapter.js';
import { createAmpCrypto } from './crypto/index.js';
import { ampErrorFromResponse } from './errors.js';
import {
  type EncryptKeyStorage,
  defaultEncryptKeyStorage,
  resolveDeviceEncryptKey,
} from './crypto/keystore.js';
import type { AmpCrypto, KeyPair } from './crypto/types.js';
import type {
  AmpItemMeta,
  AmpMember,
  AmpQueryOpts,
  BlobRef,
  CitationRef,
  LoginCredentials,
  SubscriptionEvent,
  TagResolution,
  TxOp,
  TxResult,
  UploadOpts,
  WalletChallenge,
  WithdrawOpts,
  WithdrawReason,
} from './types.js';

export interface AmpWebClientOpts {
  vaultUrl: string;       // e.g. https://my-amp-node:5193
  planetTag: string;      // the planet this client reads/writes by default

  /**
   * Where the member's device-local EncryptKey is held for BYOK seal/open.
   * Defaults to IndexedDB in the browser and an in-memory store elsewhere.
   * Inject a custom store (e.g. an OS keychain bridge) to override.
   */
  encryptKeyStorage?: EncryptKeyStorage;
}

/** The wire Item shape returned by list/read endpoints (webapi.Item). */
interface WireItem extends AmpItemMeta {
  value: Record<string, unknown>;
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

  constructor(opts: AmpWebClientOpts) {
    this.vaultUrl = opts.vaultUrl.replace(/\/$/, '');
    this.planetTag = opts.planetTag;
    this.keyStorage = opts.encryptKeyStorage ?? defaultEncryptKeyStorage();
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
      ...(item.value as Record<string, unknown>),
      _itemID: item._itemID,
      _editID: item._editID,
      _fromID: item._fromID,
      _updatedAt: item._updatedAt,
      _withdrawn: item._withdrawn,
    } as T & AmpItemMeta;
  }

  // ── Auth ──────────────────────────────────────────────────────────

  async getWalletChallenge(address: string): Promise<WalletChallenge> {
    const resp = await fetch(`${this.vaultUrl}/api/v1/login/wallet/challenge?address=${encodeURIComponent(address)}`);
    if (!resp.ok) throw await ampErrorFromResponse(resp);
    return resp.json();
  }

  async login(credentials: LoginCredentials): Promise<AmpMember> {
    const data = await this.apiFetch<{
      sessionToken: string;
      expiresAt: number;
      member: AmpMember;
    }>('/login', {
      method: 'POST',
      body: JSON.stringify(credentials),
    });

    this.sessionToken = data.sessionToken;
    this.member = {
      ...data.member,
      planetID: data.member.planetID || this.planetTag,
    };

    // Install the member's device-local EncryptKey so seal/open work without
    // an out-of-band setEncryptKey call.  Best-effort: a storage failure leaves
    // BYOK uninstalled (seal/open then throw a clear "no EncryptKey" error)
    // rather than failing the login itself.
    await this.installDeviceEncryptKey(this.member.id);

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
    if (this.sessionToken) {
      await fetch(this.apiUrl('/logout'), {
        method: 'POST',
        headers: this.headers(),
      }).catch(() => {});
    }
    this.sessionToken = null;
    this.member = null;
    this.crypto.setEncryptKey(null);
    this.disconnectWs();
    this.authListeners.forEach(cb => cb(null));
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
    const out = await this.apiFetch<{ items: WireItem[]; hasMore: boolean; next?: string }>(
      this.itemsPath(channel, attr) + (qs ? `?${qs}` : ''),
    );
    return {
      data: (out.items ?? []).map(item => this.mapItem<T>(item)),
      hasMore: out.hasMore,
      next: out.next,
    };
  }

  async tx(ops: TxOp[], planetTag?: string): Promise<TxResult[]> {
    const out = await this.apiFetch<{ txID: string; results: TxResult[] }>('/tx', {
      method: 'POST',
      body: JSON.stringify({ ops, planetTag }),
    });
    return out.results ?? [];
  }

  async create(channel: string, attr: string, value: Record<string, unknown>): Promise<string> {
    const out = await this.apiFetch<{ results: TxResult[] }>(
      this.itemsPath(channel, attr),
      { method: 'POST', body: JSON.stringify(value) },
    );
    return out.results?.[0]?.itemID ?? '';
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
    await this.apiFetch(this.itemsPath(channel, attr, itemID, 'withdraw'), {
      method: 'POST',
      body: JSON.stringify(opts),
    });
  }

  // ── Tag resolution ────────────────────────────────────────────────

  async resolveTag(expr: string): Promise<TagResolution> {
    return this.apiFetch<TagResolution>(`/tag/resolve?expr=${encodeURIComponent(expr)}`);
  }

  async resolveTags(exprs: string[]): Promise<TagResolution[]> {
    const out = await this.apiFetch<{ results: TagResolution[] }>('/tag/resolve', {
      method: 'POST',
      body: JSON.stringify({ exprs }),
    });
    return out.results ?? [];
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

  async resolveMedia(blob: BlobRef): Promise<BlobRef> {
    return this.apiFetch<BlobRef>('/media/resolve', {
      method: 'POST',
      body: JSON.stringify({ blob }),
    });
  }

  async mediaUrl(blobRefID: string): Promise<string> {
    return `${this.vaultUrl}/www/${encodeURIComponent(blobRefID)}`;
  }

  // ── Citations ─────────────────────────────────────────────────────

  citation(ref: CitationRef): CitationRef {
    return { ...ref };
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
    this.wsSend({ type: 'subscribe', channel, attr });

    return () => {
      subs!.delete(callback);
      if (subs!.size === 0) {
        this.wsSubscriptions.delete(key);
        this.wsSend({ type: 'unsubscribe', channel, attr });
      }
    };
  }

  // ── WebSocket internals ───────────────────────────────────────────

  private connectWs(): void {
    if (this.ws) return;
    // No WebSocket in SSR / non-browser hosts — REST stays usable; live
    // subscriptions resume on a client that has one.
    if (typeof WebSocket === 'undefined') return;

    const protocol = this.vaultUrl.startsWith('https') ? 'wss' : 'ws';
    const host = this.vaultUrl.replace(/^https?:\/\//, '');
    const url = `${protocol}://${host}/ws?token=${encodeURIComponent(this.sessionToken ?? '')}`;

    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      for (const key of this.wsSubscriptions.keys()) {
        const [channel, attr] = key.split(':');
        this.wsSend({ type: 'subscribe', channel, attr });
      }
    };

    this.ws.onmessage = (evt) => {
      let frame: {
        type: string;
        channel?: string;
        attr?: string;
        itemID?: string;
        editID?: string;
        fromID?: string;
        value?: Record<string, unknown>;
        updatedAt?: string;
        reason?: string;
        rationale?: string;
        subject?: string;
        delegation?: CitationRef;
      };
      try {
        frame = JSON.parse(evt.data as string);
      } catch {
        return;
      }
      const event = frameToEvent(frame);
      if (!event) return;
      const subs = this.wsSubscriptions.get(`${frame.channel}:${frame.attr}`);
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
}

/** Decode a flat webapi.SubscribeFrame into a typed SubscriptionEvent. */
function frameToEvent(frame: {
  type: string;
  itemID?: string;
  editID?: string;
  fromID?: string;
  value?: Record<string, unknown>;
  updatedAt?: string;
  reason?: string;
  rationale?: string;
  subject?: string;
  delegation?: CitationRef;
}): SubscriptionEvent | null {
  if (!frame.itemID) return null;
  switch (frame.type) {
    case 'update':
      return {
        type: 'update',
        itemID: frame.itemID,
        value: frame.value ?? {},
        editID: frame.editID ?? '',
        fromID: frame.fromID ?? '',
        updatedAt: frame.updatedAt,
      };
    case 'delete':
      return { type: 'delete', itemID: frame.itemID, editID: frame.editID, fromID: frame.fromID };
    case 'withdraw':
      return {
        type: 'withdraw',
        itemID: frame.itemID,
        editID: frame.editID,
        fromID: frame.fromID,
        reason: (frame.reason ?? 'Retracted') as WithdrawReason,
        rationale: frame.rationale,
        subject: frame.subject,
        delegation: frame.delegation,
      };
    default:
      return null;
  }
}
