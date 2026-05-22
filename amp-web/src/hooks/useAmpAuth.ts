import { useCallback, useState } from 'react';
import { useAmpContext } from '../provider';
import type { AmpAuth, LoginCredentials } from '../types';

export function useAmpAuth(): AmpAuth {
  const { adapter, member, setMember } = useAmpContext();
  const [loading, setLoading] = useState(false);

  const login = useCallback(async (credentials: LoginCredentials) => {
    setLoading(true);
    try {
      const result = await adapter.login(credentials);
      setMember(result);
    } finally {
      setLoading(false);
    }
  }, [adapter, setMember]);

  const logout = useCallback(async () => {
    setLoading(true);
    try {
      await adapter.logout();
      setMember(null);
    } finally {
      setLoading(false);
    }
  }, [adapter, setMember]);

  return {
    member,
    isAuthenticated: member !== null,
    loading,
    login,
    logout,
  };
}
