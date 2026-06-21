import { useEffect, useState } from 'react';
import { useAmpContext } from '../provider.js';
import type { AmpMediaResult } from '../types.js';

/**
 * useAmpMedia resolves a blob UID to a streamable URL via the caller-carries-the-
 * Tag path (POST /api/v1/media/resolve), falling back to the direct /www/{UID}
 * URL if resolve is unavailable.  Pass the result `url` to <img>/<video>/<audio>.
 */
export function useAmpMedia(blobUID: string, planetTag?: string): AmpMediaResult {
  const { adapter } = useAmpContext();
  const [url, setUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [contentType, setContentType] = useState<string | null>(null);
  const [byteSize, setByteSize] = useState<number | null>(null);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!blobUID) {
      setUrl(null);
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);

    adapter.resolveMedia({ UID: blobUID }, planetTag)
      .then((blob) => {
        if (cancelled) return;
        setUrl(blob.URI ?? adapter.mediaUrl(blobUID));
        setContentType(blob.ContentType ?? null);
        setByteSize(blob.I ?? null);
        setLoading(false);
      })
      .catch(() => {
        // Resolve unavailable — fall back to the direct stream URL.
        if (cancelled) return;
        setUrl(adapter.mediaUrl(blobUID));
        setLoading(false);
      });

    return () => { cancelled = true; };
  }, [adapter, blobUID, planetTag]);

  return { url, loading, contentType, byteSize, error };
}
