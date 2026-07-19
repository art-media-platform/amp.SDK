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

  // Pagination cursor — always the last SERVER-FETCHED page boundary.  Kept
  // apart from the row cache: a WS-appended row must never become the cursor
  // (pages after it would be skipped or duplicated).
  const cursorRef = useRef<string | undefined>(undefined);

  // Serialize opts so a change to limit/after/planetTag re-creates `fetch`
  // (and thus refetches) instead of being captured once through the ref.
  const optsKey = JSON.stringify(opts ?? {});

  const fetch = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await adapter.query<T>(channel, attr, optsRef.current);
      setData(result.data);
      setHasMore(result.hasMore);
      cursorRef.current = result.next ?? result.data[result.data.length - 1]?._ItemID;
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [adapter, channel, attr, optsKey]);

  // Stable handle for the subscription callback — refetching on reconnect must
  // not tear down and re-create the subscription itself.
  const fetchRef = useRef(fetch);
  fetchRef.current = fetch;

  // Initial fetch
  useEffect(() => { fetch(); }, [fetch]);

  // Live subscription — merge events into local cache
  useEffect(() => {
    // A per-call planetTag that differs from the client's default planet is a
    // cross-planet read.  The WS always binds the SESSION planet (SKILL §4.5),
    // so a live subscription here would merge foreign rows into these results:
    // cross-planet rows stay fetch-only — refetch to observe changes.
    const planetTag = optsRef.current?.planetTag;
    if (planetTag && planetTag !== (adapter.defaultPlanetTag ?? '')) {
      return;
    }
    return adapter.subscribe(channel, attr, (event: SubscriptionEvent) => {
      if (event.type === 'reconnect') {
        // The WS dropped and re-opened; frames during the gap are lost.
        void fetchRef.current();
        return;
      }
      // Single-item query: events for OTHER items on this attr never merge.
      const itemID = optsRef.current?.itemID;
      if (itemID && event.type !== 'error' && event.ItemID !== itemID) {
        return;
      }
      if (event.type === 'update') {
        setData(prev => {
          const idx = prev.findIndex(row => row._ItemID === event.ItemID);
          const updated = {
            ...event.Value,
            _ItemID: event.ItemID,
            _EditID: event.EditID,
            _FromID: event.FromID,
            // The frame's timestamp, never the local clock — _UpdatedAt is
            // derived from the item's tag.UID server-side (SKILL §4.2).
            _UpdatedAt: event.UpdatedAt ?? new Date().toISOString(),
          } as Row;
          if (idx >= 0) {
            const next = [...prev];
            next[idx] = updated;
            return next;
          }
          // Insert in _ItemID byte order — the documented list order (§4.2);
          // a tail append would break it for any UID below the last row.
          const at = prev.findIndex(row => row._ItemID > event.ItemID);
          if (at < 0) return [...prev, updated];
          return [...prev.slice(0, at), updated, ...prev.slice(at)];
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
  }, [adapter, channel, attr, optsKey]);

  const loadMore = useCallback(async () => {
    const cursor = cursorRef.current;
    if (!hasMore || !cursor) return;
    setLoading(true);
    try {
      const result = await adapter.query<T>(channel, attr, {
        ...optsRef.current,
        after: cursor,
      });
      setData(prev => {
        // Merge by _ItemID (a WS event may have already delivered a row from
        // this page), keeping the cache in _ItemID byte order throughout.
        const merged = new Map(prev.map(row => [row._ItemID, row] as const));
        for (const row of result.data) merged.set(row._ItemID, row);
        return [...merged.values()].sort((a, b) =>
          a._ItemID < b._ItemID ? -1 : a._ItemID > b._ItemID ? 1 : 0);
      });
      setHasMore(result.hasMore);
      cursorRef.current =
        result.next ?? result.data[result.data.length - 1]?._ItemID ?? cursor;
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [adapter, channel, attr, hasMore]);

  return { data, loading, error, refetch: fetch, hasMore, loadMore };
}
