/**
 * useAmpMedia's resolve round (resolveMediaURL): the Tag posted to
 * /media/resolve must carry the cabinet's ContentTypeRaw — the server
 * publishes and serves under the POSTED Tag's ContentType() (app.www
 * api.www.go AssetForBlob → www.webService.go PublishAsset), so a UID-only
 * post yields /www/{uid}.plain served as text/plain.
 */

import { describe, expect, it } from 'vitest';

import type { BlobRef } from '../types.js';
import { mediaTagFor, resolveMediaURL } from './useAmpMedia.js';

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
