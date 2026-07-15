/**
 * AmpAdminClient — the Node-only operator-tier module: issue rides the admin
 * Bearer and matches the Go wire contract; the constructor refuses to run in
 * a browser context (the operator Bearer must never enter browser JS).
 */

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { AmpAdminClient } from './admin.js';
import { AmpErrorCode } from './errors.js';

const VAULT = 'http://127.0.0.1:5193';

const realFetch = globalThis.fetch;

let calls: { url: string; auth: string | null; body: Record<string, unknown> }[] = [];
let issueStatus = 201;

beforeEach(() => {
  calls = [];
  issueStatus = 201;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const headers = (init?.headers ?? {}) as Record<string, string>;
    calls.push({
      url: String(input),
      auth: headers['Authorization'] ?? null,
      body: init?.body ? JSON.parse(String(init.body)) : {},
    });
    if (issueStatus !== 201) {
      return new Response(
        JSON.stringify({ Code: 'Forbidden', Message: 'not an admin' }),
        { status: issueStatus, headers: { 'content-type': 'application/json' } },
      );
    }
    return new Response(
      JSON.stringify({ MemberID: '06efvw-28sb2-00124-8j248-j248j', Email: 'bob@example.com' }),
      { status: 201, headers: { 'content-type': 'application/json' } },
    );
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe('issueEmailCredential', () => {
  it('POSTs Email + Password with the admin Bearer and returns the seeded MemberID', async () => {
    const admin = new AmpAdminClient({ vaultUrl: `${VAULT}/`, adminToken: 'op-bearer' });
    const out = await admin.issueEmailCredential({ email: 'bob@example.com', password: 'initial-secret-9' });

    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe(`${VAULT}/api/v1/admin/credentials/email/issue`);
    expect(calls[0].auth).toBe('Bearer op-bearer');
    expect(calls[0].body).toEqual({ Email: 'bob@example.com', Password: 'initial-secret-9' });
    expect(out.MemberID).toBe('06efvw-28sb2-00124-8j248-j248j');
    expect(out.Email).toBe('bob@example.com');
  });

  it('a non-admin Bearer surfaces as a typed AmpError', async () => {
    issueStatus = 403;
    const admin = new AmpAdminClient({ vaultUrl: VAULT, adminToken: 'member-bearer' });
    await expect(admin.issueEmailCredential({ email: 'b@x.com', password: 'p' }))
      .rejects.toMatchObject({ status: 403, code: AmpErrorCode.Forbidden });
  });
});

describe('browser refusal', () => {
  it('constructor throws when a browser window+document are present', () => {
    const g = globalThis as { window?: unknown; document?: unknown };
    const hadWindow = 'window' in g ? g.window : undefined;
    const hadDocument = 'document' in g ? g.document : undefined;
    g.window = {};
    g.document = {};
    try {
      expect(() => new AmpAdminClient({ vaultUrl: VAULT, adminToken: 'op-bearer' }))
        .toThrow(/server-side tooling only/);
    } finally {
      if (hadWindow === undefined) delete g.window; else g.window = hadWindow;
      if (hadDocument === undefined) delete g.document; else g.document = hadDocument;
    }
  });
});
