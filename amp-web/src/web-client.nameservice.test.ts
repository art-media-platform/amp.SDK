/**
 * NameService client methods (SKILL §4.6): resolve / search / federationPeers
 * against the live wire contract, replayed from the SAME golden fixtures the
 * Go side round-trips (amp/webapi/testdata/vault.json, drift_test.go).
 */

import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { AmpError, AmpErrorCode } from './errors.js';
import { AmpWebClient } from './web-client.js';
import type { FederationPeerEntry, ResolveResponse, SearchMatch } from './types.js';

const HERE = dirname(fileURLToPath(import.meta.url));
const TESTDATA_CANDIDATES = [
  join(HERE, '..', '..', 'amp', 'webapi', 'testdata'),
  join(HERE, '..', 'webapi', 'testdata'),
];
const testdataDir = TESTDATA_CANDIDATES.find(dir => existsSync(dir)) ?? '';

const vaultGolden = JSON.parse(
  readFileSync(join(testdataDir, 'vault.json'), 'utf8'),
) as {
  ResolveRequest: { FQDN: string };
  ResolveResponse: ResolveResponse;
  SearchRequest: { Query: string; Limit: number };
  SearchResponse: { Matches: SearchMatch[] };
  FederationPeersResponse: { Peers: FederationPeerEntry[] };
};

const VAULT = 'http://127.0.0.1:5193';

const realFetch = globalThis.fetch;

interface Call {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: unknown;
}

let calls: Call[] = [];
let respond: (url: string) => Response;

function stubFetch(): void {
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    calls.push({
      url,
      method: init?.method ?? 'GET',
      headers: (init?.headers ?? {}) as Record<string, string>,
      body: typeof init?.body === 'string' ? JSON.parse(init.body) : null,
    });
    return respond(url);
  }) as typeof fetch;
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function client(): AmpWebClient {
  return new AmpWebClient({ vaultUrl: VAULT, planetTag: '' });
}

function lastCall(): Call {
  return calls[calls.length - 1];
}

beforeEach(() => {
  calls = [];
  respond = () => jsonResponse({});
  stubFetch();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe('resolve() — POST /api/v1/resolve, anonymous exact-match FQDN resolution', () => {
  it('POSTs the golden ResolveRequest body {FQDN} and returns the golden ResolveResponse verbatim', async () => {
    respond = () => jsonResponse(vaultGolden.ResolveResponse);
    const res = await client().resolve(vaultGolden.ResolveRequest.FQDN);

    expect(lastCall().url).toBe(`${VAULT}/api/v1/resolve`);
    expect(lastCall().method).toBe('POST');
    expect(lastCall().body).toEqual(vaultGolden.ResolveRequest);
    expect(res).toEqual(vaultGolden.ResolveResponse);
    expect(res.TrustState).toBe('Verified');
  });

  it('sends no Authorization header signed-out (the anonymous posture, DD-name-service §3.2)', async () => {
    respond = () => jsonResponse(vaultGolden.ResolveResponse);
    await client().resolve('plan.tools');
    expect(lastCall().headers['Authorization']).toBeUndefined();
  });

  it('maps the no-record 404 to a typed AmpError NotFound', async () => {
    respond = () => jsonResponse({ Code: 'NotFound', Message: 'no federation names that FQDN' }, 404);
    const err = await client().resolve('nobody.example').catch((thrown: unknown) => thrown);
    expect(err).toBeInstanceOf(AmpError);
    expect((err as AmpError).status).toBe(404);
    expect((err as AmpError).code).toBe(AmpErrorCode.NotFound);
  });
});

describe('search() — POST /api/v1/search, Bearer-gated ranked discovery', () => {
  it('POSTs the golden SearchRequest body {Query, Limit} and unwraps Matches', async () => {
    respond = () => jsonResponse(vaultGolden.SearchResponse);
    const matches = await client().search(
      vaultGolden.SearchRequest.Query,
      vaultGolden.SearchRequest.Limit,
    );

    expect(lastCall().url).toBe(`${VAULT}/api/v1/search`);
    expect(lastCall().method).toBe('POST');
    expect(lastCall().body).toEqual(vaultGolden.SearchRequest);
    expect(matches).toEqual(vaultGolden.SearchResponse.Matches);
    expect(matches[0].FQDN).toBe('plan.tools');
  });

  it('omits Limit from the body when the caller passes none', async () => {
    respond = () => jsonResponse({ Matches: [] });
    await client().search('plan');
    expect(lastCall().body).toEqual({ Query: 'plan' });
  });

  it('normalizes an absent Matches to an empty array', async () => {
    respond = () => jsonResponse({});
    expect(await client().search('nothing')).toEqual([]);
  });
});

describe('federationPeers() — GET /api/v1/federation/peers, Bearer-gated', () => {
  it('rides the required ?federation= UID and unwraps the golden Peers', async () => {
    respond = () => jsonResponse(vaultGolden.FederationPeersResponse);
    const fedID = vaultGolden.FederationPeersResponse.Peers[0].FederationID;
    const peers = await client().federationPeers(fedID);

    expect(lastCall().url).toBe(
      `${VAULT}/api/v1/federation/peers?federation=${encodeURIComponent(fedID)}`,
    );
    expect(lastCall().method).toBe('GET');
    expect(peers).toEqual(vaultGolden.FederationPeersResponse.Peers);
    expect(peers[0].Label).toBe('Plan Federation');
  });

  it('normalizes an absent Peers to an empty array', async () => {
    respond = () => jsonResponse({});
    expect(await client().federationPeers('06e-fvw28sb400-248j248j24-8j2')).toEqual([]);
  });
});
