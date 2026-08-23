import { useEffect, useState } from 'react';
import { AmpError } from '../errors.js';
import { useAmpContext } from '../provider.js';
import type { AmpResolveResult } from '../types.js';

/**
 * useAmpResolve maps a registered FQDN to the planet that serves it via the
 * anonymous NameService resolve (POST /api/v1/resolve — SKILL §4.6), so a
 * deep-link landing can dial + pin before any session exists.  A missing
 * record is an answer, not a failure: resolution === null with no error.
 * Never auto-follow a non-Verified / Ambiguous TrustState — render it as such.
 */
export function useAmpResolve(fqdn: string): AmpResolveResult {
  const { adapter } = useAmpContext();
  const [resolution, setResolution] = useState<AmpResolveResult['resolution']>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!fqdn) {
      setResolution(null);
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);

    adapter.resolve(fqdn)
      .then((res) => {
        if (cancelled) return;
        setResolution(res);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        if (err instanceof AmpError && err.status === 404) {
          setResolution(null);   // no federation names that FQDN
        } else {
          setError(err instanceof Error ? err : new Error(String(err)));
        }
        setLoading(false);
      });

    return () => { cancelled = true; };
  }, [adapter, fqdn]);

  return { resolution, loading, error };
}
