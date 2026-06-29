/**
 * window.__amp — the engine-agnostic host bridge (AD-app-forums.md §6.4-6.5).
 *
 * When the SPA runs embedded in the Unity host, the host injects `window.__amp` — a
 * tiny shim wrapping its native channel (never `window.vuplex` directly, so the SPA
 * survives the Vuplex→FOSS engine swap) — carrying the SSO memberToken and a routing
 * table of the verbs it handles natively.  The SDK diverts those verbs (today: the
 * member's self-signed forums reply) to the host so the member's OWN key signs the
 * write; reads and every other write stay on the custodial HTTP path.  A pure browser
 * has no `window.__amp` and is unaffected.
 */

import type { TxResult } from './types.js';

export interface AmpEmbed {
  embed: boolean;
  memberToken?: string;
  member?: string;
  bridgeVerbs?: Record<string, string>;             // verbURL -> host message type
  toHost(json: string): void;                        // page → host (host-injected glue)
  onHostMessage(cb: (json: string) => void): void;   // host → page (host-injected glue)
}

declare global {
  interface Window {
    __amp?: AmpEmbed;
  }
}

interface Pending {
  resolve: (value: TxResult[]) => void;
  reject: (err: Error) => void;
}

/**
 * EmbedBridge correlates the SPA's diverted invoke() calls with the host's replies.
 * One in-flight map; the host echoes a SPA-minted `id` and answers `${type}.result`.
 */
export class EmbedBridge {
  private pending = new Map<string, Pending>();
  private seq = 0;

  constructor(private host: AmpEmbed) {
    host.onHostMessage(raw => this.onMessage(raw));
  }

  /** True when the host advertises a native handler for this verb. */
  routes(verbURL: string): boolean {
    return !!this.host.bridgeVerbs?.[verbURL];
  }

  /** Divert one verb op to the host; resolves when the host replies `${type}.result`. */
  invoke(verbURL: string, value: unknown, planetTag?: string, topicID?: string): Promise<TxResult[]> {
    const type = this.host.bridgeVerbs![verbURL];
    const id = `${type}#${++this.seq}`;
    const p = new Promise<TxResult[]>((resolve, reject) => this.pending.set(id, { resolve, reject }));
    this.host.toHost(JSON.stringify({ type, id, planetTag, topicID, verbURL, value }));
    return p;
  }

  private onMessage(raw: string): void {
    let msg: Record<string, unknown>;
    try {
      msg = JSON.parse(raw);
    } catch {
      return;
    }
    const type = msg.type;

    // A fresh memberToken pushed by the host (re-mint after auth-expired).
    if (type === 'session' && typeof msg.token === 'string') {
      this.host.memberToken = msg.token;
      return;
    }

    if (typeof type !== 'string' || !type.endsWith('.result')) {
      return;
    }
    const waiter = this.pending.get(msg.id as string);
    if (!waiter) {
      return;
    }
    this.pending.delete(msg.id as string);
    if (msg.ok) {
      waiter.resolve([{ ItemID: typeof msg.itemID === 'string' ? msg.itemID : '', EditID: '' }]);
    } else {
      waiter.reject(new Error(typeof msg.error === 'string' ? msg.error : 'host declined the write'));
    }
  }
}
