// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Navigate } from 'react-router';
import { useAuth } from './useAuth';

export default function RootPage() {
  const isAuthenticated = useAuth((s) => s.isAuthenticated);
  const needsSetup = useAuth((s) => s.needsSetup);

  if (needsSetup) {
    return <Navigate to="/setup" replace />;
  }

  if (isAuthenticated) {
    return <Navigate to="/dashboard" replace />;
  }

  return <Navigate to="/login" replace />;
}
