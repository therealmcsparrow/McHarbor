// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useDemoStatus } from '@resources/hooks/useDemoMode';

export function SuppressedInDemo({ children }: { children: ReactNode }) {
  const currentStatus = useDemoStatus();
  if (currentStatus.data?.active) return null;
  return <>{children}</>;
}
