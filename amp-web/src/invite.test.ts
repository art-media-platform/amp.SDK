/**
 * Invite wire-mapping unit tests — stub fetch, assert each /api/v1/invite/*
 * request shape (camelCase opts → PascalCase wire) and response parse.  No
 * portal required.
 */
import { afterEach, describe, expect, it, vi } from 'vitest';

import { AmpWebClient } from './web-client.js';

function stubFetch(body: unknown, status = 200) {
  const seen: { url: string; init: RequestInit }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit) => {
    seen.push({ url, init });
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    });
  }));
  return seen;
}

describe('invites', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('acceptInvite POSTs InviteText + Passphrase', async () => {
    const seen = stubFetch({ PlanetID: 'planetUID32', MemberID: 'memberUID32' }, 201);
    const amp = new AmpWebClient({ vaultUrl: 'https://prod.plan.tools', planetTag: 'p' });
    const out = await amp.acceptInvite({ inviteText: 'https://x/invite#abc', passphrase: 'hunter2' });

    expect(out).toEqual({ PlanetID: 'planetUID32', MemberID: 'memberUID32' });
    expect(seen[0].url).toBe('https://prod.plan.tools/api/v1/invite/accept');
    expect(JSON.parse(seen[0].init.body as string)).toEqual({
      InviteText: 'https://x/invite#abc',
      Passphrase: 'hunter2',
    });
  });

  it('issueInvite maps a multi-use policy request and returns the invite ID', async () => {
    const seen = stubFetch({ PlanetID: 'p32', InviteID: 'inv32', InviteText: 'https://x/invite#body' }, 201);
    const amp = new AmpWebClient({ vaultUrl: 'https://prod.plan.tools', planetTag: 'p' });
    const out = await amp.issueInvite({
      planet: 'p32', passphrase: 'pw', maxRedemptions: 5, access: 'ReadWrite',
    });

    expect(out.InviteID).toBe('inv32');
    expect(seen[0].url).toBe('https://prod.plan.tools/api/v1/invite/issue');
    expect(JSON.parse(seen[0].init.body as string)).toEqual({
      Planet: 'p32', Passphrase: 'pw', MaxRedemptions: 5, Access: 'ReadWrite', ExpiresAt: 0, VaultAddrs: [],
    });
  });

  it('revokeInvite maps the rotate flag', async () => {
    const seen = stubFetch({ Policies: [] });
    const amp = new AmpWebClient({ vaultUrl: 'https://prod.plan.tools', planetTag: 'p' });
    await amp.revokeInvite({ planet: 'p32', inviteId: 'inv32', rotate: true });

    expect(seen[0].url).toBe('https://prod.plan.tools/api/v1/invite/revoke');
    expect(JSON.parse(seen[0].init.body as string)).toEqual({
      Planet: 'p32', InviteID: 'inv32', InviteText: '', Rotate: true,
    });
  });

  it('listInvites GETs by planet and parses policies', async () => {
    const seen = stubFetch({ Policies: [{ InviteID: 'inv32', MaxRedemptions: 2, Status: 'InviteActive' }] });
    const amp = new AmpWebClient({ vaultUrl: 'https://prod.plan.tools', planetTag: 'p' });
    const out = await amp.listInvites('p32');

    expect(out.Policies[0].InviteID).toBe('inv32');
    expect(seen[0].url).toBe('https://prod.plan.tools/api/v1/invite/list?planet=p32');
    expect(seen[0].init.method).toBe('GET');
  });
});
