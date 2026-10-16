// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ContainerInfo } from '@core/types/docker';
import { assertSuccess } from '@resources/utils/api-mutation';
import { RUNBOOKS_KEY } from '@resources/hooks/useRunbooks';
import type { RunbookTargetOption, TargetKind } from '@resources/types/runbooks';

export type TargetSelectOption = { value: string; label: string; description?: string };

type StackRow = { name: string; services?: unknown[] };

function envParams(envId: string): Record<string, string> {
  return envId ? { env: envId } : {};
}

async function fetchTargets(kind: TargetKind, envId: string, cluster: string): Promise<TargetSelectOption[]> {
  if (kind === 'container') {
    const rows =
      assertSuccess(await api.get<ContainerInfo[]>('/containers', { all: 'true', ...envParams(envId) })) ?? [];
    return rows.map((row) => {
      const itemName = (row.Names[0] ?? row.Id).replace(/^[/]/, '');
      return { value: itemName, label: itemName, description: row.Image };
    });
  }
  if (kind === 'stack') {
    const rows = assertSuccess(await api.get<StackRow[]>('/stacks', envParams(envId))) ?? [];
    return rows.map((row) => ({ value: row.name, label: row.name }));
  }
  const rows =
    assertSuccess(
      await api.get<RunbookTargetOption[]>('/runbooks/targets', {
        kind,
        ...envParams(envId),
        ...(cluster ? { cluster } : {}),
      }),
    ) ?? [];
  return rows.map((row) => ({ value: row.id, label: row.name, description: row.detail || undefined }));
}

export function useRunbookTargets(kind: TargetKind | undefined, envId: string, cluster = '') {
  return useQuery({
    queryKey: [...RUNBOOKS_KEY, 'targets', kind ?? '', envId, cluster],
    enabled: kind !== undefined && kind !== 'environment',
    staleTime: 30_000,
    queryFn: () => fetchTargets(kind as TargetKind, envId, cluster),
  });
}
