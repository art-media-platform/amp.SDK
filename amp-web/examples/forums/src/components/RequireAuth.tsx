import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { useAmpAuth } from '@art-media-platform/web';

/** Gates a write route: anonymous visitors are redirected to /login. */
export function RequireAuth({ children }: { children: ReactNode }) {
  const { isAuthenticated, loading } = useAmpAuth();
  if (loading) return <div className="forums-empty">Loading…</div>;
  if (!isAuthenticated) return <Navigate to="/login" replace />;
  return <>{children}</>;
}
