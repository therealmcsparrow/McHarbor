// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, type ReactNode } from 'react';
import { useIdleLogout } from '@resources/hooks/useIdleLogout';
import { useAuth } from './useAuth';

export function AuthProvider({ children }: { children: ReactNode }) {
  const checkSession = useAuth((s) => s.checkSession);

  // Mounted here rather than in AppLayout so the countdown covers every
  // authenticated surface, including the ones that render outside the
  // main shell (forced password change, permission-denied pages).
  useIdleLogout();

  useEffect(() => {
    void checkSession();
  }, [checkSession]);

  return <>{children}</>;
}
