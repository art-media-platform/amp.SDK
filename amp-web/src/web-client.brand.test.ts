/**
 * getBrand() — read-only substrate Brand consumption (SKILL §10) over the
 * standard items rail, surfacing NamedBy + the resolver's TrustState verdict
 * on the Brand's claimed AppDomain.
 *
 * The TrustState verdicts replayed here are the server's: the three-state
 * back-edge outcomes are pinned Go-side by app.nameservice
 * TestBuildResolutionTrustState (matching NamedBy → Verified, mismatched →
 * Refuted, no live record → Unchecked); this side asserts the client passes
 * each verdict through unaltered.
 */

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { AmpError } from './errors.js';
import { AmpWebClient } from './web-client.js';
import type { Brand } from './types.js';

const VAULT = 'http://127.0.0.1:5193';

// The head-node Brand anchor (verified against stdlib/tag UID.Base32():
// amp.HeadNodeID{0,0x37} and std.Attr.Brand {0xB70889689791764B,0xB554FC786AE221AE}).
const BRAND_ITEM_PATH =
  '/api/v1/channels/000-0000000000-0000000000-01r/attrs/amp.Brand/items/5r1-24qj5wjft5-vbp7wg1pf4-8ef';

const FED_ALPHA = '607-2p143cp5n0-kvyrrnsq4f-39h';

const realFetch = globalThis.fetch;

let calls: { url: string; body: unknown }[] = [];
let brandStatus = 200;
let brandValue: Brand = {};
let resolveStatus = 200;
let resolveTrust = 'Verified';

function stubFetch(): void {
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    calls.push({ url, body: typeof init?.body === 'string' ? JSON.parse(init.body) : null });

    const json = (body: unknown, status: number) => new Response(JSON.stringify(body), {
      status,
      headers: { 'content-type': 'application/json' },
    });

    if (url.includes('/resolve')) {
      if (resolveStatus !== 200) {
        return json({ Code: 'NotFound', Message: 'no federation names that FQDN' }, resolveStatus);
      }
      return json({
        FQDN: (brandValue.Identity?.AppDomain ?? ''),
        PlanetID: '4e4-weyzwpm83r-ugwdd985vr-mj7',
        AnsweredBy: FED_ALPHA,
        TrustState: resolveTrust,
        PinPrecedence: false,
        Ambiguous: false,
        Hops: 0,
      }, 200);
    }
    if (brandStatus !== 200) {
      return json({ Code: 'NotFound', Message: 'item not found' }, brandStatus);
    }
    return json({
      _ItemID: '5r1-24qj5wjft5-vbp7wg1pf4-8ef',
      _EditID: '0er-0000000000-0000000000-000',
      _FromID: '0uf-45v0x7vmx2-3s8w8mgcgc-dhz',
      _UpdatedAt: '2026-08-15T00:00:00Z',
      Value: brandValue,
    }, 200);
  }) as typeof fetch;
}

function client(planetTag = ''): AmpWebClient {
  return new AmpWebClient({ vaultUrl: VAULT, planetTag });
}

beforeEach(() => {
  calls = [];
  brandStatus = 200;
  brandValue = {
    Identity: {
      AppName: 'Maplable Shares',
      OrgName: 'Maplable',
      AppDomain: 'shares.maplable.com',
      NamedBy: { UID: FED_ALPHA },
    },
  };
  resolveStatus = 200;
  resolveTrust = 'Verified';
  stubFetch();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe('getBrand() addresses the head-node Brand anchor over the items rail', () => {
  it('GETs (HeadNodeID, amp.Brand, its own UID) with the per-call planetTag', async () => {
    await client().getBrand('maplable-shares');
    expect(calls[0].url).toBe(`${VAULT}${BRAND_ITEM_PATH}?planetTag=maplable-shares`);
  });

  it('falls back to the constructor planetTag when no per-call tag is given', async () => {
    await client('share-planet').getBrand();
    expect(calls[0].url).toContain('planetTag=share-planet');
  });
});

describe('getBrand() verdicts pass the server TrustState through unaltered (Go truth: TestBuildResolutionTrustState)', () => {
  it('matching NamedBy back-edge → Verified, with NamedBy + resolution surfaced', async () => {
    const res = await client().getBrand('maplable-shares');
    expect(res).not.toBeNull();
    expect(res!.trustState).toBe('Verified');
    expect(res!.namedBy).toBe(FED_ALPHA);
    expect(res!.brand.Identity?.AppName).toBe('Maplable Shares');
    expect(res!.resolution?.AnsweredBy).toBe(FED_ALPHA);
  });

  it('mismatched back-edge → Refuted (a squatter claim renders as such, never Verified)', async () => {
    resolveTrust = 'Refuted';
    const res = await client().getBrand('maplable-shares');
    expect(res!.trustState).toBe('Refuted');
  });

  it('no live record for the claimed AppDomain (resolve 404) → the cold Unchecked verdict', async () => {
    resolveStatus = 404;
    const res = await client().getBrand('maplable-shares');
    expect(res!.trustState).toBe('Unchecked');
    expect(res!.resolution).toBeUndefined();
  });

  it('a Brand claiming no AppDomain skips resolve entirely and stays Unchecked', async () => {
    brandValue = { Identity: { AppName: 'Home', NamedBy: { UID: FED_ALPHA } } };
    const res = await client().getBrand('maplable-shares');
    expect(res!.trustState).toBe('Unchecked');
    expect(res!.namedBy).toBe(FED_ALPHA);
    expect(calls).toHaveLength(1);
  });
});

describe('getBrand() absence and failure surfaces', () => {
  it('resolves null for a planet with no Brand authored (naked home planet, item 404)', async () => {
    brandStatus = 404;
    expect(await client().getBrand('naked-home')).toBeNull();
  });

  it('rethrows a non-404 items failure as the typed AmpError', async () => {
    brandStatus = 403;
    const err = await client().getBrand('forbidden').catch((thrown: unknown) => thrown);
    expect(err).toBeInstanceOf(AmpError);
    expect((err as AmpError).status).toBe(403);
  });

  it('surfaces an unset NamedBy as an empty namedBy', async () => {
    brandValue = { Identity: { AppName: 'Loner' } };
    const res = await client().getBrand('loner');
    expect(res!.namedBy).toBe('');
  });
});
