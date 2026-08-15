import { useEffect, useState } from 'react';
import { useAmpContext } from '../provider.js';
import type { AmpBrandResult } from '../types.js';

/**
 * useAmpBrand reads a planet's substrate Brand (display-only — SKILL §10)
 * with its NamedBy back-edge and the resolver's TrustState verdict on the
 * claimed AppDomain, for the planet header / picker tile / Verified badge.
 * brand === null with no error = the planet carries no Brand (naked home
 * planet) — render the epoch label instead.  Omit planetTag to read the
 * client's default planet.
 */
export function useAmpBrand(planetTag?: string): AmpBrandResult {
  const { adapter } = useAmpContext();
  const [brand, setBrand] = useState<AmpBrandResult['brand']>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    adapter.getBrand(planetTag)
      .then((res) => {
        if (cancelled) return;
        setBrand(res);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err : new Error(String(err)));
        setLoading(false);
      });

    return () => { cancelled = true; };
  }, [adapter, planetTag]);

  return { brand, loading, error };
}
