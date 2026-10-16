// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import type { ContainerInfo, ContainerInspect } from '@core/types/docker';
import { needsHostSelf } from '@core/utils/protection';
import { useHostSelfKey } from '@resources/hooks/useHostSelfKey';
import { useEnvironmentStore } from '@resources/stores/environment';

/**
 * The permission gates for actions on one container: `lifecycle` for start, stop, edit,
 * rename, move and remove, `exec` for a shell, the debug shell and file writes, which also
 * reach a container that mounts McHarbor's host.
 */
export function useHostSelfGate(container: ContainerInfo | ContainerInspect | null | undefined) {
  const lifecycle = useHostSelfKey(needsHostSelf(container, 'lifecycle'));
  const exec = useHostSelfKey(needsHostSelf(container, 'exec'));
  return { lifecycle, exec };
}

/**
 * useHostSelfGate for a tab that only has the id. It subscribes to the detail page's
 * cached inspect without fetching: the tabs render under that page, which loads it.
 */
export function useContainerHostSelfGate(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const { data } = useQuery<ContainerInspect>({ queryKey: ['container', envId, containerId], enabled: false });
  return useHostSelfGate(data);
}
