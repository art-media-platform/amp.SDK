/**
 * AmpProvider — React context provider; inject your AmpWebClient (or any
 * AmpAdapter) via the `client` prop.
 */

import React, { createContext, useContext, useEffect, useState } from 'react';
import type { AmpAdapter } from './adapter.js';
import type { AmpMember } from './types.js';

interface AmpContextValue {
  adapter: AmpAdapter;
  member: AmpMember | null;
  setMember: (member: AmpMember | null) => void;
}

const AmpContext = createContext<AmpContextValue | null>(null);

export interface AmpProviderProps {
  client: AmpAdapter;
  children: React.ReactNode;
}

export function AmpProvider({ client, children }: AmpProviderProps) {
  const [member, setMember] = useState<AmpMember | null>(() => client.getSession());

  useEffect(() => {
    return client.onAuthChange(setMember);
  }, [client]);

  return (
    <AmpContext.Provider value={{ adapter: client, member, setMember }}>
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
