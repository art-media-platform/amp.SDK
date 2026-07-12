/**
 * AmpProvider — React context provider; inject your AmpWebClient (or any
 * AmpAdapter) via the `client` prop.  On mount it rehydrates a persisted
 * session (adapter.restoreSession) so a page reload lands authenticated.
 */

import React, { createContext, useContext, useEffect, useState } from 'react';
import type { AmpAdapter } from './adapter.js';
import type { AmpMember } from './types.js';

interface AmpContextValue {
  adapter: AmpAdapter;
  member: AmpMember | null;
  /** True while the initial restoreSession() pass is in flight. */
  restoring: boolean;
  setMember: (member: AmpMember | null) => void;
}

const AmpContext = createContext<AmpContextValue | null>(null);

export interface AmpProviderProps {
  client: AmpAdapter;
  children: React.ReactNode;
}

export function AmpProvider({ client, children }: AmpProviderProps) {
  const [member, setMember] = useState<AmpMember | null>(() => client.getSession());
  const [restoring, setRestoring] = useState<boolean>(() => client.getSession() === null);

  useEffect(() => {
    const unsubscribe = client.onAuthChange(setMember);
    // Rehydrate a persisted session — onAuthChange delivers the member.  A
    // transport failure just ends the restoring state signed-out; the
    // persisted record survives for the next load (see restoreSession).
    client.restoreSession()
      .catch(() => null)
      .finally(() => setRestoring(false));
    return unsubscribe;
  }, [client]);

  return (
    <AmpContext.Provider value={{ adapter: client, member, restoring, setMember }}>
      {children}
    </AmpContext.Provider>
  );
}

/** Internal hook — retrieves the adapter and auth state from context. */
export function useAmpContext(): AmpContextValue {
  const ctx = useContext(AmpContext);
  if (!ctx) {
    throw new Error('useAmpContext must be used within an <AmpProvider>');
  }
  return ctx;
}

/**
 * useAmpClient — the adapter (your AmpWebClient) for imperative calls that no
 * dedicated hook covers: login challenges (getWalletChallenge / getDIDChallenge),
 * server-side tag resolution (resolveTag / resolveTags), ad-hoc query.
 * Must be used within an <AmpProvider>.
 */
export function useAmpClient(): AmpAdapter {
  return useAmpContext().adapter;
}
