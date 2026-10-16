// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { HostServicesPanel } from '@resources/components/host/HostServicesPanel';

type EnvironmentServicesTabProps = {
  envId: string;
};

export function EnvironmentServicesTab({ envId }: EnvironmentServicesTabProps) {
  const host = useMemo<HostTarget>(() => ({ kind: 'environment', envId }), [envId]);
  return <HostServicesPanel host={host} tableId="environment-host-services" />;
}
