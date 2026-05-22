// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, type ReactNode } from 'react';
import { useAuth } from './useAuth';

export function AuthProvider({ children }: { children: ReactNode }) {
  const checkSession = useAuth((s) => s.checkSession);

  useEffect(() => {
    checkSession();
  }, [checkSession]);

  return <>{children}</>;
}
