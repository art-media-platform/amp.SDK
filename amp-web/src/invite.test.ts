/**
 * acceptInvite wire-mapping unit test — stubs fetch, asserts the
 * POST /api/v1/invite/accept request shape (camelCase opts → PascalCase wire)
 * and the { PlanetID, MemberID } response parse.  No portal required.
 */
import { afterEach, describe, expect, it, vi } from 'vitest';

import { AmpWebClient } from './web-client.js';

describe('acceptInvite', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('POSTs InviteText + Passphrase and returns the joined planet + member', async () => {
    const seen: { url: string; init: RequestInit }[] = [];
    vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit) => {
      seen.push({ url, init });
      return new Response(JSON.stringify({ PlanetID: 'planetUID32', MemberID: 'memberUID32' }), {
        status: 201,
        headers: { 'Content-Type': 'application/json' },
      });
    }));

    const amp = new AmpWebClient({ vaultUrl: 'https://prod.plan.tools', planetTag: 'p' });
    const out = await amp.acceptInvite({ inviteText: 'amp-invite-v1:abc', passphrase: 'hunter2' });

    expect(out).toEqual({ PlanetID: 'planetUID32', MemberID: 'memberUID32' });
    expect(seen).toHaveLength(1);
    expect(seen[0].url).toBe('https://prod.plan.tools/api/v1/invite/accept');
    expect(seen[0].init.method).toBe('POST');
    expect(JSON.parse(seen[0].init.body as string)).toEqual({
      InviteText: 'amp-invite-v1:abc',
      Passphrase: 'hunter2',
    });
  });
});
