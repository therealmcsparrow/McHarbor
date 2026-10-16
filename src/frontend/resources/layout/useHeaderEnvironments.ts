// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';

type EnvSummary = {
  id: string;
  name: string;
  connectionType: string;
  isDefault: boolean;
  dockerVersion: string | null;
  scheduledUpdateCheckEnabled: boolean;
  automaticImagePruningEnabled: boolean;
  trackContainerEventsEnabled: boolean;
  collectContainerMetricsEnabled: boolean;
  highlightContainerChangesEnabled: boolean;
  dockerDiskUsageNotificationsEnabled: boolean;
  dockerDiskUsageThresholdPercent: number;
  agentStatus?: string | null;
  lastConnected?: string | null;
};

export function useHeaderEnvironments() {
  const currentId = useEnvironmentStore((s) => s.currentId);
  const setCurrentId = useEnvironmentStore((s) => s.setCurrentId);
  const setEnvironments = useEnvironmentStore((s) => s.setEnvironments);

  const environmentsQuery = useQuery({
    queryKey: ['environments'],
    queryFn: () => api.get<EnvSummary[]>('/environments').then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
  // The topbar switcher drives every Docker page — containers, images,
  // volumes, stacks. Since Kubernetes moved out to its own plugin (which
  // carries its own cluster picker), every environment here is a Docker host
  // and there is nothing left to filter out.
  const dockerEnvironments = useMemo(() => environmentsQuery.data ?? [], [environmentsQuery.data]);

  useEffect(() => {
    if (environmentsQuery.data) {
      setEnvironments(dockerEnvironments);
    }
  }, [environmentsQuery.data, dockerEnvironments, setEnvironments]);

  useEffect(() => {
    if (dockerEnvironments.length === 0) {
      return;
    }
    // Make the topbar environment selector always render with a
    // concrete environment selected. Without this, a stale
    // persisted currentId (empty or pointing at a deleted env)
    // leaves the selector with no matching option. Prefer the
    // explicit default env when present; fall back to the first
    // item in the list.
    const current = dockerEnvironments.find((env) => env.id === currentId);
    if (current) {
      return;
    }
    const preferred = dockerEnvironments.find((env) => env.isDefault) ?? dockerEnvironments[0];
    if (preferred && preferred.id !== currentId) {
      setCurrentId(preferred.id);
    }
  }, [dockerEnvironments, currentId, setCurrentId]);
}
