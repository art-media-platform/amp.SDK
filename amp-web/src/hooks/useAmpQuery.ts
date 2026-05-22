import { useCallback, useEffect, useRef, useState } from 'react';
import { useAmpContext } from '../provider';
import type { AmpItemMeta, AmpQueryOpts, AmpQueryResult, SubscriptionEvent } from '../types';

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
  }, [adapter, channel, attr]);

  // Initial fetch
  useEffect(() => { fetch(); }, [fetch]);

  // Live subscription — merge events into local cache
  useEffect(() => {
    return adapter.subscribe(channel, attr, (event: SubscriptionEvent) => {
      if (event.type === 'update') {
        setData(prev => {
          const idx = prev.findIndex(row => row._itemID === event.itemID);
          const updated = {
            ...event.value,
            _itemID: event.itemID,
            _editID: event.editID,
            _fromID: event.fromID,
            _updatedAt: new Date().toISOString(),
          } as Row;
          if (idx >= 0) {
            const next = [...prev];
            next[idx] = updated;
            return next;
          }
          return [...prev, updated];
        });
      } else if (event.type === 'delete') {
        setData(prev => prev.filter(row => row._itemID !== event.itemID));
      } else if (event.type === 'withdraw') {
        // The original item stays visible; surface the withdrawal companion.
        setData(prev => prev.map(row => row._itemID === event.itemID
          ? { ...row, _withdrawn: {
              reason: event.reason,
              rationale: event.rationale,
              withdrawnAt: new Date().toISOString(),
              withdrawnBy: event.fromID ?? '',
              subject: event.subject,
              delegation: event.delegation,
            } }
          : row));
      }
    });
  }, [adapter, channel, attr]);

  const loadMore = useCallback(async () => {
    if (!hasMore || data.length === 0) return;
    const lastID = data[data.length - 1]._itemID;
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
