/**
 * AmpProvider — React context provider with adapter injection.
 */

import React, { createContext, useContext, useEffect, useState } from 'react';
import type { AmpAdapter } from './adapter';
import type { AmpMember } from './types';

interface AmpContextValue {
  adapter: AmpAdapter;
  member: AmpMember | null;
  setMember: (member: AmpMember | null) => void;
}

const AmpContext = createContext<AmpContextValue | null>(null);

export interface AmpProviderProps {
  adapter: AmpAdapter;
  children: React.ReactNode;
}

export function AmpProvider({ adapter, children }: AmpProviderProps) {
  const [member, setMember] = useState<AmpMember | null>(() => adapter.getSession());

  useEffect(() => {
    return adapter.onAuthChange(setMember);
  }, [adapter]);

  return (
    <AmpContext.Provider value={{ adapter, member, setMember }}>
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
