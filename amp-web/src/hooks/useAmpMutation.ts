import { useCallback, useState } from 'react';
import { useAmpContext } from '../provider';
import type { AmpMutationResult, TxOp, TxResult, WithdrawOpts } from '../types';

export function useAmpMutation(): AmpMutationResult {
  const { adapter } = useAmpContext();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  // run wraps a mutation with shared loading/error bookkeeping.
  const run = useCallback(async <T>(op: () => Promise<T>): Promise<T> => {
    setLoading(true);
    setError(null);
    try {
      return await op();
    } catch (err) {
      const wrapped = err instanceof Error ? err : new Error(String(err));
      setError(wrapped);
      throw wrapped;
    } finally {
      setLoading(false);
    }
  }, []);

  const tx = useCallback((ops: TxOp[], planetTag?: string): Promise<TxResult[]> =>
    run(() => adapter.tx(ops, planetTag)), [adapter, run]);

  const create = useCallback((channel: string, attr: string, value: Record<string, unknown>): Promise<string> =>
    run(() => adapter.create(channel, attr, value)), [adapter, run]);

  const upsert = useCallback((channel: string, attr: string, itemID: string, value: Record<string, unknown>): Promise<void> =>
    run(() => adapter.upsert(channel, attr, itemID, value)), [adapter, run]);

  const remove = useCallback((channel: string, attr: string, itemID: string): Promise<void> =>
    run(() => adapter.remove(channel, attr, itemID)), [adapter, run]);

  const withdraw = useCallback((channel: string, attr: string, itemID: string, opts: WithdrawOpts): Promise<void> =>
    run(() => adapter.withdraw(channel, attr, itemID, opts)), [adapter, run]);

  return { tx, create, upsert, remove, withdraw, loading, error };
}
