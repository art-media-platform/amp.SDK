import { useCallback, useState } from 'react';
import { useAmpContext } from '../provider.js';
import type { AmpAuth, LoginCredentials } from '../types.js';

export function useAmpAuth(): AmpAuth {
  const { adapter, member } = useAmpContext();
  const [loading, setLoading] = useState(false);

  const login = useCallback(async (credentials: LoginCredentials) => {
    setLoading(true);
    try {
      // adapter.login fires onAuthChange — the provider is the single source of
      // member state — and returns the member so a caller can use it directly.
      return await adapter.login(credentials);
    } finally {
      setLoading(false);
    }
  }, [adapter]);

  const logout = useCallback(async () => {
    setLoading(true);
    try {
      await adapter.logout();
    } finally {
      setLoading(false);
    }
  }, [adapter]);

  return {
    member,
    isAuthenticated: member !== null,
    loading,
    login,
    logout,
  };
}
