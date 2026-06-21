import { useCallback, useEffect, useRef, useState } from 'react';
import { useAmpContext } from '../provider.js';
import type { AmpItemMeta, AmpQueryOpts, AmpQueryResult, SubscriptionEvent } from '../types.js';

export function useAmpQuery<T>(
  channel: string,
  attr: string,
  opts?: AmpQueryOpts,
): AmpQueryResult<T> {
  const { adapter } = useAmpContext();

  type Row = T & AmpItemMeta;
  const [data, setData] = useState<Row[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [hasMore, setHasMore] = useState(false);

  const optsRef = useRef(opts);
  optsRef.current = opts;

  // Serialize opts so a change to limit/after/filter/planetTag re-creates `fetch`
  // (and thus refetches) instead of being captured once through the ref.
  const optsKey = JSON.stringify(opts ?? {});

  const fetch = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await adapter.query<T>(channel, attr, optsRef.current);
      setData(result.data);
      setHasMore(result.hasMore);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [adapter, channel, attr, optsKey]);

  // Initial fetch
  useEffect(() => { fetch(); }, [fetch]);

  // Live subscription — merge events into local cache
  useEffect(() => {
    return adapter.subscribe(channel, attr, (event: SubscriptionEvent) => {
      if (event.type === 'update') {
        setData(prev => {
          const idx = prev.findIndex(row => row._ItemID === event.ItemID);
          const updated = {
            ...event.Value,
            _ItemID: event.ItemID,
            _EditID: event.EditID,
            _FromID: event.FromID,
            _UpdatedAt: new Date().toISOString(),
          } as Row;
          if (idx >= 0) {
            const next = [...prev];
            next[idx] = updated;
            return next;
          }
          return [...prev, updated];
        });
      } else if (event.type === 'delete') {
        setData(prev => prev.filter(row => row._ItemID !== event.ItemID));
      } else if (event.type === 'withdraw') {
        // The original item stays visible; surface the withdrawal companion.
        setData(prev => prev.map(row => row._ItemID === event.ItemID
          ? { ...row, _Withdrawn: {
              ...event.Withdraw,
              WithdrawnAt: event.Withdraw.WithdrawnAt ?? new Date().toISOString(),
              WithdrawnBy: event.Withdraw.WithdrawnBy ?? event.FromID ?? '',
            } }
          : row));
      } else if (event.type === 'error') {
        // A server-side subscribe rejection — surface it instead of silently
        // never delivering.
        setError(new Error(event.Error));
      }
    });
  }, [adapter, channel, attr]);

  const loadMore = useCallback(async () => {
    if (!hasMore || data.length === 0) return;
    const lastID = data[data.length - 1]._ItemID;
    setLoading(true);
    try {
      const result = await adapter.query<T>(channel, attr, {
        ...optsRef.current,
        after: lastID,
      });
      setData(prev => [...prev, ...result.data]);
      setHasMore(result.hasMore);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [adapter, channel, attr, hasMore, data]);

  return { data, loading, error, refetch: fetch, hasMore, loadMore };
}
