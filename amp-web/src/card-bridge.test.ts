/**
 * AmpBridge IDL guard — the card-author type against the served bridge's
 * contract (AD-app-www.md §6.5): `onReady(cb)` is the card's cue that the host
 * attached, fires at once when already attached, and hands the bridge to the
 * callback.  The implementation ships with the node (`/amp/bridge.js`), so the
 * host below is a reference of that attach semantic, typed as AmpBridge — a
 * verb the type lacks fails `tsc`, and the source assertion fails `vitest`.
 */

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import type { AmpBridge } from './card-bridge.js';

const HERE = dirname(fileURLToPath(import.meta.url));
const IDL_SOURCE = readFileSync(join(HERE, 'card-bridge.ts'), 'utf8');

// The §6.5 spelling, byte-for-byte: the doc IDL and this type must not drift.
const ON_READY_SIGNATURE = 'onReady(cb: (amp: AmpBridge) => void): void;';

/** A host with the served bridge's attach semantic: callbacks queue until attach, then fire once. */
function referenceHost(): { amp: AmpBridge; attach(): void } {
  let attached = false;
  let readyCbs: Array<(amp: AmpBridge) => void> = [];
  const inert = () => { throw new Error('not exercised'); };
  const amp: AmpBridge = {
    member: null,
    onReady(cb) {
      if (attached) cb(amp);
      else readyCbs.push(cb);
    },
    read: inert, list: inert, tx: inert, write: inert, remove: inert, withdraw: inert,
    subscribe: inert, upload: inert, resolveMedia: inert, seal: inert, open: inert,
    navigate: inert, back: inert, setTitle: inert, focus: inert, onBack: inert,
    onFocusChanged: inert, onScroll: inert, submit: inert, setVar: inert, onVar: inert,
  };
  return {
    amp,
    attach() {
      attached = true;
      amp.member = { ID: 'm1', DisplayName: 'card member', PlanetID: 'p1' };
      const cbs = readyCbs;
      readyCbs = [];
      for (const cb of cbs) cb(amp);
    },
  };
}

describe('AmpBridge.onReady', () => {
  it('is declared in the §6.5 spelling under Identity', () => {
    expect(IDL_SOURCE).toContain(ON_READY_SIGNATURE);
    const identity = IDL_SOURCE.indexOf('// ── Identity ──');
    const data = IDL_SOURCE.indexOf('// ── Data ──');
    const onReady = IDL_SOURCE.indexOf(ON_READY_SIGNATURE);
    expect(identity).toBeGreaterThan(-1);
    expect(onReady).toBeGreaterThan(identity);
    expect(onReady).toBeLessThan(data);
  });

  it('defers the callback until the host attaches, then hands over the bridge with member set', () => {
    const host = referenceHost();
    const seen: AmpBridge[] = [];
    host.amp.onReady(amp => seen.push(amp));
    expect(seen).toHaveLength(0);
    expect(host.amp.member).toBeNull();
    host.attach();
    expect(seen).toHaveLength(1);
    expect(seen[0]).toBe(host.amp);
    expect(seen[0].member?.ID).toBe('m1');
  });

  it('fires at once when already attached, and each callback once', () => {
    const host = referenceHost();
    host.attach();
    let calls = 0;
    host.amp.onReady(() => { calls++; });
    expect(calls).toBe(1);
    host.attach();
    expect(calls).toBe(1);
  });
});
