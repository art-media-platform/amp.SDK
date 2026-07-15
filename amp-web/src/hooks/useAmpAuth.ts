import { useCallback, useState } from 'react';
import { useAmpContext } from '../provider.js';
import type { AmpAuth, ClaimAccountOpts, LoginCredentials, RedeemEmailOpts } from '../types.js';

export function useAmpAuth(): AmpAuth {
  const { adapter, member, restoring } = useAmpContext();
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

  const recoverEmail = useCallback(async (email: string) => {
    setLoading(true);
    try {
      await adapter.recoverEmail(email);
    } finally {
      setLoading(false);
    }
  }, [adapter]);

  // redeem/claim mint a session, so both fire onAuthChange like login does.
  const redeemEmail = useCallback(async (opts: RedeemEmailOpts) => {
    setLoading(true);
    try {
      return await adapter.redeemEmail(opts);
    } finally {
      setLoading(false);
    }
  }, [adapter]);

  const claimAccount = useCallback(async (opts: ClaimAccountOpts) => {
    setLoading(true);
    try {
      return await adapter.claimAccount(opts);
    } finally {
      setLoading(false);
    }
  }, [adapter]);

  return {
    member,
    isAuthenticated: member !== null,
    // `loading` folds in the initial session restore, so an existing
    // "if (loading) show spinner" gate is reload-flash-free with no change.
    loading: loading || restoring,
    restoring,
    login,
    logout,
    recoverEmail,
    redeemEmail,
    claimAccount,
  };
}
