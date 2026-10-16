// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Navigate } from 'react-router';
import { useAuth } from './useAuth';
import { useResolvedLayout } from '@resources/hooks/useLayoutMode';
import { useDatabaseSetupStatus } from './useDatabaseSetupStatus';

export default function RootPage() {
  const isAuthenticated = useAuth((s) => s.isAuthenticated);
  const needsSetup = useAuth((s) => s.needsSetup);
  const layout = useResolvedLayout();
  // Only asked while there is no account: once one exists the endpoint
  // refuses, and the answer would not change where this sends anyone.
  const dbStatus = useDatabaseSetupStatus(needsSetup);

  if (needsSetup) {
    // The database question comes before the first admin account, because
    // that account is the first row McHarbor writes and asking afterwards
    // would mean either discarding it or copying it. While the status is
    // still loading, hold rather than guess — landing on the admin form and
    // being bounced back a moment later is worse than a blank frame.
    if (dbStatus.isLoading) return null;
    const currentStatus = dbStatus.data;
    if (currentStatus && !currentStatus.configured && !currentStatus.envPinned) {
      return <Navigate to="/setup/database" replace />;
    }
    return <Navigate to="/setup" replace />;
  }

  if (isAuthenticated) {
    return <Navigate to={layout === 'oncall' ? '/oncall' : '/dashboard'} replace />;
  }

  return <Navigate to="/login" replace />;
}
