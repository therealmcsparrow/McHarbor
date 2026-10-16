// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { Navigate } from 'react-router';
import { useAuth } from './useAuth';

export const MFA_ENROLLMENT_PATH = '/mfa-enrollment';

export function MFAEnrollmentGuard({ children }: { children: ReactNode }) {
  const isAuthenticated = useAuth((s) => s.isAuthenticated);
  const enrollmentRequired = useAuth((s) => s.mfaEnrollmentRequired);

  if (isAuthenticated && enrollmentRequired) {
    return <Navigate to={MFA_ENROLLMENT_PATH} replace />;
  }
  return <>{children}</>;
}
