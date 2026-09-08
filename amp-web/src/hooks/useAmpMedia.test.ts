/**
 * useAmpMedia's resolve round (resolveMediaURL): the Tag posted to
 * /media/resolve must carry the cabinet's ContentTypeRaw — the server
 * publishes and serves under the POSTED Tag's ContentType() (app.www
 * api.www.go AssetForBlob → www.webService.go PublishAsset) — and the URI
 * the server answers is carried VERBATIM: a member resolve's URI holds the
 * media token (/www/{UID}.{ext}?t=…, app.www www.media_auth.go), the only
 * credential /www/ accepts, so the hook builds no URL of its own.
 */

import { describe, expect, it } from 'vitest';

import type { BlobRef } from '../types.js';
import { mediaTagFor, refreshMediaURL, resolveMediaURL } from './useAmpMedia.js';

const UID = '06e-fvw28sb600-36dtm6dtm6-dtm';
const CABINET: BlobRef = { UID, ContentTypeRaw: 'video/mp4', I: 10, Units: 2 };

/**
 * A stand-in for the server's media plane: /media/resolve publishes under the
 * posted Tag's ContentType() and answers a token URL bound to the session's
 * generation and a lifetime bucket; /www/ serves only a published asset whose
 * token is unexpired and at the member's current generation.
 */
function mediaServer(opts: { resolveFails?: boolean } = {}) {
  const posted: BlobRef[] = [];
  const published = new Set<string>();
  let generation = 0;
  let bucket = 0;
  let sessionRevoked = false;
  const assetPath = (blob: BlobRef): string => {
    const subtype = (blob.ContentTypeRaw || 'text/plain').toLowerCase().split('/')[1] ?? '';
    return `/www/${blob.UID}${subtype ? '.' + subtype : ''}`;
  };
  const tokenFor = (path: string): string => `exp${bucket}.sess.member.gen${generation}.mac-${path.length}`;
  return {
    posted,
    async resolveMedia(blob: BlobRef): Promise<BlobRef> {
      posted.push(blob);
      if (opts.resolveFails) throw new Error('resolve unavailable');
      if (sessionRevoked) throw new Error('401 auth required');
      const path = assetPath(blob);
      published.add(path);
      return { ...blob, URI: `${path}?t=${tokenFor(path)}` };
    },
    /** The server's idle expiry fires: the asset is unpublished (its token is still valid). */
    idleOut(): void { published.clear(); },
    /** The token lifetime's bucket passes: every outstanding token is expired. */
    expireTokens(): void { bucket++; },
    /** admin/session/revoke: the member's generation bumps and the session is gone. */
    revokeMember(): void { generation++; sessionRevoked = true; },
    /** What a <video> range request sees at the URL. */
    serve(url: string | null): number {
      if (!url) return 0;
      const [path, query] = url.split('?');
      const token = new URLSearchParams(query ?? '').get('t');
      if (!published.has(path!) || token !== tokenFor(path!)) return 404;
      return 200;
    },
  };
}

describe('useAmpMedia resolve round', () => {
  it('posts the cabinet Tag whole — ContentTypeRaw, I, Units — and carries the answered URI verbatim, token included', async () => {
    const server = mediaServer();
    const res = await resolveMediaURL(server, CABINET);
    expect(server.posted).toEqual([CABINET]);
    expect(res.url).toMatch(new RegExp(`^/www/${UID}\\.mp4\\?t=`));
    expect(res.error).toBeNull();
    expect(res.contentType).toBe('video/mp4');
    expect(res.byteSize).toBe(10);
    expect(server.serve(res.url)).toBe(200);
    // The path alone — what the hook once rebuilt — is a dead URL.
    expect(server.serve(res.url!.split('?')[0]!)).toBe(404);
  });

  it('a failed resolve yields no URL and the error — nothing is rebuilt client-side', async () => {
    const server = mediaServer({ resolveFails: true });
    const res = await resolveMediaURL(server, CABINET);
    expect(res.url).toBeNull();
    expect(res.error?.message).toBe('resolve unavailable');
    expect(res.contentType).toBe('video/mp4');
    expect(res.byteSize).toBe(10);
  });

  it('a resolve that answers no URI is an error, not a bare path', async () => {
    const server = { async resolveMedia(blob: BlobRef): Promise<BlobRef> { return { ...blob }; } };
    const res = await resolveMediaURL(server, CABINET);
    expect(res.url).toBeNull();
    expect(res.error?.message).toContain('no URI');
  });

  it('a bare UID degrades to a UID-only Tag (server ⇒ text/plain)', async () => {
    const server = mediaServer();
    const res = await resolveMediaURL(server, mediaTagFor(UID));
    expect(server.posted).toEqual([{ UID }]);
    expect(res.url).toMatch(new RegExp(`^/www/${UID}\\.plain\\?t=`));
    expect(res.contentType).toBeNull();
  });
});

/**
 * The 404 paths the element's onError → refresh loop covers: the asset idled
 * out (the same URL serves again inside the token bucket), the token expired
 * (a fresh token URL), and the member revoked (the resolve itself fails).
 */
describe('useAmpMedia 404 path (idle expiry, token expiry, revocation)', () => {
  it('after the server idles the asset out, refresh re-resolves and the SAME url serves again', async () => {
    const server = mediaServer();
    const first = await resolveMediaURL(server, CABINET);
    expect(server.serve(first.url)).toBe(200);

    server.idleOut();
    expect(server.serve(first.url)).toBe(404); // the paused player's next range request

    const again = await refreshMediaURL(server, CABINET);
    expect(server.posted).toHaveLength(2);
    expect(again.url).toBe(first.url);
    expect(server.serve(again.url)).toBe(200);
  });

  it('after the token lifetime passes, refresh answers a FRESH token URL that serves', async () => {
    const server = mediaServer();
    const first = await resolveMediaURL(server, CABINET);
    expect(server.serve(first.url)).toBe(200);

    server.expireTokens();
    expect(server.serve(first.url)).toBe(404); // the player's next range request past the lifetime

    const again = await refreshMediaURL(server, CABINET);
    expect(again.url).not.toBe(first.url);
    expect(again.url!.split('?')[0]).toBe(first.url!.split('?')[0]); // same path, new token
    expect(server.serve(again.url)).toBe(200);
    expect(server.serve(first.url)).toBe(404);
  });

  it('after the member is revoked, the URL dies and refresh surfaces the resolve failure', async () => {
    const server = mediaServer();
    const first = await resolveMediaURL(server, CABINET);
    expect(server.serve(first.url)).toBe(200);

    server.revokeMember();
    expect(server.serve(first.url)).toBe(404); // the generation bump refuses the outstanding token

    const again = await refreshMediaURL(server, CABINET);
    expect(again.url).toBeNull();
    expect(again.error?.message).toContain('401');
  });
});
