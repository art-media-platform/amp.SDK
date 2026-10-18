import { useCallback, useEffect, useState } from 'react';
import type { AmpAdapter } from '../adapter.js';
import { useAmpContext } from '../provider.js';
import type { Address, AmpMediaResult, BlobRef } from '../types.js';

/**
 * The Tag the hook carries to /media/resolve: the cabinet's BlobRef as given.
 * A bare UID becomes a UID-only Tag — degraded: the server resolves an absent
 * ContentTypeRaw to text/plain, outside the inline media families, so the
 * blob serves as an application/octet-stream download.
 */
export function mediaTagFor(blob: string | BlobRef): BlobRef {
  return typeof blob === 'string' ? { UID: blob } : blob;
}

/**
 * The Tag the hook posts, built from the effect-key scalars: UID,
 * ContentTypeRaw, I, Units and the Tag's own Cell (the BlobRef cell its upload
 * wrote, which the client's resolveMedia sends when no cell argument does).
 */
export function postedMediaTag(fields: Pick<BlobRef, 'UID' | 'ContentTypeRaw' | 'I' | 'Units' | 'Cell'>): BlobRef {
  const posted: BlobRef = { UID: fields.UID };
  if (fields.ContentTypeRaw !== undefined) posted.ContentTypeRaw = fields.ContentTypeRaw;
  if (fields.I !== undefined) posted.I = fields.I;
  if (fields.Units !== undefined) posted.Units = fields.Units;
  if (fields.Cell !== undefined) posted.Cell = fields.Cell;
  return posted;
}

/** What useAmpMedia settles to once resolve answers or fails. */
export interface MediaResolution {
  url: string | null;
  contentType: string | null;
  byteSize: number | null;
  error: Error | null;
}

/**
 * One resolve round: POST the Tag (caller-carries-the-Tag — the server
 * publishes under the Tag's ContentType, api.www.go AssetForBlob) and carry
 * the answered URI verbatim — a member resolve's URI holds the media token
 * (/www/{UID}.{ext}?t=…), the only credential the media plane accepts, so
 * there is no URL to build client-side: a failed resolve is a null url plus
 * the error.  Metadata comes from the answer, else the Tag.  cell names the
 * item's BlobRef cell (a packed Address) so a sealed blob resolves on a node
 * that did not seal it.
 */
export async function resolveMediaURL(
  adapter: Pick<AmpAdapter, 'resolveMedia'>, tag: BlobRef, planetTag?: string, cell?: Address,
): Promise<MediaResolution> {
  try {
    const blob = await adapter.resolveMedia(tag, planetTag, cell);
    if (!blob.URI) {
      throw new Error('media resolve answered no URI');
    }
    return {
      url: blob.URI,
      contentType: blob.ContentTypeRaw ?? tag.ContentTypeRaw ?? null,
      byteSize: blob.I ?? tag.I ?? null,
      error: null,
    };
  } catch (err) {
    return {
      url: null,
      contentType: tag.ContentTypeRaw ?? null,
      byteSize: tag.I ?? null,
      error: err instanceof Error ? err : new Error(String(err)),
    };
  }
}

/**
 * The 404 path: the URL stops serving when the asset idles out server-side
 * (no request for DefaultAssetIdleExpire — a paused player), when its media
 * token passes its lifetime, or when the member's sessions are revoked.  One
 * resolve round answers a URL that serves again — the same URL inside the
 * token lifetime's bucket, a fresh token past it; the caller then reloads
 * the element.  A revoked session fails the resolve itself (401 → the
 * client drops its session) and surfaces as the error.
 */
export async function refreshMediaURL(
  adapter: Pick<AmpAdapter, 'resolveMedia'>, tag: BlobRef, planetTag?: string, cell?: Address,
): Promise<MediaResolution> {
  return resolveMediaURL(adapter, tag, planetTag, cell);
}

/**
 * useAmpMedia resolves a blob to a streamable URL via the caller-carries-the-
 * Tag path (POST /api/v1/media/resolve); the URL is the server's, token
 * included.  Pass the cabinet's BlobRef (its ContentTypeRaw decides the
 * served MIME type + extension); a bare UID is accepted, degraded to an
 * octet-stream download.  Pass the result `url` to <img>/<video>/<audio>, and bind
 * `refresh` to the element's onError — past the server's idle expiry or the
 * token lifetime the URL 404s until a refresh re-resolves it.  A sealed blob
 * resolves by its BlobRef cell: the BlobRef's own Cell (the upload's) rides
 * along by default; pass cell (a packed Address) to name another.
 */
export function useAmpMedia(blob: string | BlobRef, planetTag?: string, cell?: Address): AmpMediaResult {
  const { adapter } = useAmpContext();
  const [url, setUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [contentType, setContentType] = useState<string | null>(null);
  const [byteSize, setByteSize] = useState<number | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const [generation, setGeneration] = useState(0);

  // Effect keys are the Tag's scalar fields, not the BlobRef identity: an
  // inline object literal is a fresh identity every render.
  const tag = mediaTagFor(blob);
  const { UID: uid, ContentTypeRaw: contentTypeRaw, I: byteLen, Units: units, Cell: tagCell } = tag;

  useEffect(() => {
    if (!uid) {
      setUrl(null);
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);

    const posted = postedMediaTag({ UID: uid, ContentTypeRaw: contentTypeRaw, I: byteLen, Units: units, Cell: tagCell });
    resolveMediaURL(adapter, posted, planetTag, cell).then((res) => {
      if (cancelled) return;
      setUrl(res.url);
      setContentType(res.contentType);
      setByteSize(res.byteSize);
      setError(res.error);
      setLoading(false);
    });

    return () => { cancelled = true; };
  }, [adapter, uid, contentTypeRaw, byteLen, units, tagCell, planetTag, cell]);

  const refresh = useCallback(async () => {
    if (!uid) return;
    const posted = postedMediaTag({ UID: uid, ContentTypeRaw: contentTypeRaw, I: byteLen, Units: units, Cell: tagCell });
    const res = await refreshMediaURL(adapter, posted, planetTag, cell);
    setUrl(res.url);
    setContentType(res.contentType);
    setByteSize(res.byteSize);
    setError(res.error);
    setGeneration(prev => prev + 1);
  }, [adapter, uid, contentTypeRaw, byteLen, units, tagCell, planetTag, cell]);

  return { url, loading, contentType, byteSize, error, refresh, generation };
}
