// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useHostLinks } from './useCompute';

/** Sorts a host's links into the bridges and uplinks the network form offers, and reads the picked uplink's state. */
export function useNetworkUplinks(environmentId: string, uplinkLink: string) {
  const { data: links, isLoading } = useHostLinks(environmentId || undefined);

  const bridges = useMemo(() => (links ?? []).filter((link) => link.kind === 'bridge'), [links]);
  // Anything a managed network could route out through. Loopback and the
  // veth/tap devices McHarbor itself creates are not candidates.
  const uplinks = useMemo(
    () => (links ?? []).filter((link) => link.kind === 'physical' || link.kind === 'bond' || link.kind === 'vlan'),
    [links],
  );

  const selectedUplink = uplinks.find((link) => link.name === uplinkLink);
  const uplinkIsDefaultRoute = Boolean(selectedUplink?.hasDefaultRoute);
  const uplinkBlockers = selectedUplink?.upper ?? [];
  const uplinkIsBusy = uplinkBlockers.length > 0;

  const noHostNetwork = !isLoading && (links ?? []).length > 0 && bridges.length === 0 && uplinks.length === 0;

  return {
    isLoading,
    bridges,
    uplinks,
    uplinkIsDefaultRoute,
    uplinkBlockers,
    uplinkIsBusy,
    noHostNetwork,
  };
}
