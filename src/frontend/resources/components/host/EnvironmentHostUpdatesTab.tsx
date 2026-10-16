// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { HostUpdatesPanel } from './HostUpdatesPanel';

type EnvironmentHostUpdatesTabProps = {
  envId: string;
};

export function EnvironmentHostUpdatesTab({ envId }: EnvironmentHostUpdatesTabProps) {
  const target = useMemo<HostTarget>(() => ({ kind: 'environment', envId }), [envId]);
  return <HostUpdatesPanel target={target} />;
}
