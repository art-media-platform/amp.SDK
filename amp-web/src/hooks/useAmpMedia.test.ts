/**
 * useAmpMedia's resolve round (resolveMediaURL): the Tag posted to
 * /media/resolve must carry the cabinet's ContentTypeRaw — the server
 * publishes and serves under the POSTED Tag's ContentType() (app.www
 * api.www.go AssetForBlob → www.webService.go PublishAsset), so a UID-only
 * post yields /www/{uid}.plain served as text/plain.
 */

import { describe, expect, it } from 'vitest';

import type { BlobRef } from '../types.js';
import { mediaTagFor, refreshMediaURL, resolveMediaURL } from './useAmpMedia.js';

const UID = '06e-fvw28sb600-36dtm6dtm6-dtm';
const CABINET: BlobRef = { UID, ContentTypeRaw: 'video/mp4', I: 10, Units: 2 };

/** A stand-in for the server: publishes under the posted Tag's ContentType(), as the bridge does. */
function fakeAdapter(opts: { resolveFails?: boolean } = {}) {
  const posted: BlobRef[] = [];
  const mediaUrl = (blob: BlobRef): string => {
    const subtype = (blob.ContentTypeRaw || 'text/plain').toLowerCase().split('/')[1] ?? '';
    return `/www/${blob.UID}${subtype ? '.' + subtype : ''}`;
  };
  return {
    posted,
    mediaUrl,
    async resolveMedia(blob: BlobRef): Promise<BlobRef> {
      posted.push(blob);
      if (opts.resolveFails) throw new Error('resolve unavailable');
      return { ...blob, URI: mediaUrl(blob) };
    },
  };
}

describe('useAmpMedia resolve round', () => {
  it('posts the cabinet Tag whole — ContentTypeRaw, I, Units — to /media/resolve', async () => {
    const adapter = fakeAdapter();
    const res = await resolveMediaURL(adapter, CABINET);
    expect(adapter.posted).toEqual([CABINET]);
    expect(res.url).toBe(`/www/${UID}.mp4`);
    expect(res.contentType).toBe('video/mp4');
    expect(res.byteSize).toBe(10);
  });

  it('falls back to /www/{uid}.{ext} + the Tag metadata when resolve is unavailable', async () => {
    const adapter = fakeAdapter({ resolveFails: true });
    const res = await resolveMediaURL(adapter, CABINET);
    expect(res.url).toBe(`/www/${UID}.mp4`);
    expect(res.contentType).toBe('video/mp4');
    expect(res.byteSize).toBe(10);
  });

  it('a bare UID degrades to a UID-only Tag (server ⇒ text/plain)', async () => {
    const adapter = fakeAdapter();
    const res = await resolveMediaURL(adapter, mediaTagFor(UID));
    expect(adapter.posted).toEqual([{ UID }]);
    expect(res.url).toBe(`/www/${UID}.plain`);
    expect(res.contentType).toBeNull();
  });
});

/**
 * The 404 path: a stand-in server whose published asset idles out — its URL
 * answers 404 until the blob is resolved again (app.www www.webService.go
 * PublishAsset: the same asset ID re-published serves at the same URL).
 */
function idlingServer() {
  const adapter = fakeAdapter();
  const published = new Set<string>();
  return {
    adapter,
    resolves: adapter.posted,
    async resolveMedia(blob: BlobRef): Promise<BlobRef> {
      const out = await adapter.resolveMedia(blob);
      published.add(out.URI!);
      return out;
    },
    mediaUrl: adapter.mediaUrl,
    /** The server's idle expiry fires: the asset is unpublished. */
    idleOut(): void { published.clear(); },
    /** What a <video> range request sees at the URL. */
    serve(url: string): number { return published.has(url) ? 200 : 404; },
  };
}

describe('useAmpMedia 404 path (idle expiry)', () => {
  it('after the server idles the asset out, refresh re-resolves and the SAME url serves again', async () => {
    const server = idlingServer();
    const first = await resolveMediaURL(server, CABINET);
    expect(server.serve(first.url)).toBe(200);

    server.idleOut();
    expect(server.serve(first.url)).toBe(404); // the paused player's next range request

    const again = await refreshMediaURL(server, CABINET);
    expect(server.resolves).toHaveLength(2);
    expect(again.url).toBe(first.url);
    expect(server.serve(again.url)).toBe(200);
  });
});
