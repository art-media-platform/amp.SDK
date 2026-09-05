import { useEffect, useState } from 'react';
import type { AmpAdapter } from '../adapter.js';
import { useAmpContext } from '../provider.js';
import type { AmpMediaResult, BlobRef } from '../types.js';

/**
 * The Tag the hook carries to /media/resolve: the cabinet's BlobRef as given.
 * A bare UID becomes a UID-only Tag — degraded: the server resolves an absent
 * ContentTypeRaw to text/plain and publishes + serves the blob under it.
 */
export function mediaTagFor(blob: string | BlobRef): BlobRef {
  return typeof blob === 'string' ? { UID: blob } : blob;
}

/** What useAmpMedia settles to once resolve answers or fails. */
export interface MediaResolution {
  url: string;
  contentType: string | null;
  byteSize: number | null;
}

/**
 * One resolve round: POST the Tag (caller-carries-the-Tag — the server
 * publishes under the Tag's ContentType, api.www.go AssetForBlob), answer the
 * stream URL; when resolve is unavailable, the direct /www/{UID}.{ext} URL
 * built from the same Tag.  Metadata comes from the answer, else the Tag.
 */
export async function resolveMediaURL(
  adapter: Pick<AmpAdapter, 'resolveMedia' | 'mediaUrl'>, tag: BlobRef, planetTag?: string,
): Promise<MediaResolution> {
  try {
    const blob = await adapter.resolveMedia(tag, planetTag);
    return {
      url: blob.URI ?? adapter.mediaUrl(blob),
      contentType: blob.ContentTypeRaw ?? tag.ContentTypeRaw ?? null,
      byteSize: blob.I ?? tag.I ?? null,
    };
  } catch {
    return {
      url: adapter.mediaUrl(tag),
      contentType: tag.ContentTypeRaw ?? null,
      byteSize: tag.I ?? null,
    };
  }
}

/**
 * useAmpMedia resolves a blob to a streamable URL via the caller-carries-the-
 * Tag path (POST /api/v1/media/resolve), falling back to the direct
 * /www/{UID}.{ext} URL if resolve is unavailable.  Pass the cabinet's BlobRef
 * (its ContentTypeRaw decides the served MIME type + extension); a bare UID
 * is accepted, degraded to text/plain.  Pass the result `url` to
 * <img>/<video>/<audio>.
 */
export function useAmpMedia(blob: string | BlobRef, planetTag?: string): AmpMediaResult {
  const { adapter } = useAmpContext();
  const [url, setUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [contentType, setContentType] = useState<string | null>(null);
  const [byteSize, setByteSize] = useState<number | null>(null);
  const [error, setError] = useState<Error | null>(null);

  // Effect keys are the Tag's scalar fields, not the BlobRef identity: an
  // inline object literal is a fresh identity every render.
  const tag = mediaTagFor(blob);
  const { UID: uid, ContentTypeRaw: contentTypeRaw, I: byteLen, Units: units } = tag;

  useEffect(() => {
    if (!uid) {
      setUrl(null);
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);

    const posted: BlobRef = { UID: uid };
    if (contentTypeRaw !== undefined) posted.ContentTypeRaw = contentTypeRaw;
    if (byteLen !== undefined) posted.I = byteLen;
    if (units !== undefined) posted.Units = units;

    resolveMediaURL(adapter, posted, planetTag).then((res) => {
      if (cancelled) return;
      setUrl(res.url);
      setContentType(res.contentType);
      setByteSize(res.byteSize);
      setLoading(false);
    });

    return () => { cancelled = true; };
  }, [adapter, uid, contentTypeRaw, byteLen, units, planetTag]);

  return { url, loading, contentType, byteSize, error };
}
