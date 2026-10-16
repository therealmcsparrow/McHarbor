// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { EnvironmentListItem } from '@resources/hooks/useEnvironmentList';

export type ConnectionTarget = 'socket' | 'tcp' | 'ssh' | 'agent';

export type MigrateConnectionData = {
  targetType: ConnectionTarget;
  socketPath?: string;
  host?: string;
  port?: number;
  sshHost?: string;
  sshPort?: number;
  sshUser?: string;
  sshKey?: string;
  agentToken?: string;
  immediate?: boolean;
  skipAgentDeploy?: boolean;
  agentImage?: string;
  insecureTls?: boolean;
};

export type AgentDeployment = {
  attempted: boolean;
  deployed: boolean;
  containerId?: string;
  image?: string;
  error?: string;
};

export type MigrateConnectionResult = {
  environment: EnvironmentListItem;
  pending: boolean;
  agentToken?: string;
  agent?: AgentDeployment;
};

export type MergePlanEntry = {
  table: string;
  column: string;
  rows: number;
};

export type MergeResult = {
  sourceId: string;
  sourceName: string;
  targetId: string;
  targetName: string;
  dryRun: boolean;
  moved: MergePlanEntry[] | null;
  discarded?: MergePlanEntry[] | null;
  totalRows: number;
  defaultMoved?: boolean;
};

export type DuplicateMember = {
  id: string;
  name: string;
  connectionType: string;
  isDefault: boolean;
};

export type DuplicateGroup = {
  reason: string;
  hostname?: string;
  members: DuplicateMember[];
};

function invalidateEnvironments(queryClient: ReturnType<typeof useQueryClient>, id?: string) {
  void queryClient.invalidateQueries({ queryKey: ['environments'] });
  if (id) void queryClient.invalidateQueries({ queryKey: ['environments', id] });
  void queryClient.invalidateQueries({ queryKey: ['environment-duplicates'] });
}

export function useMigrateConnection() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: MigrateConnectionData }) =>
      api.post<MigrateConnectionResult>(`/environments/${id}/migrate`, data).then(assertSuccess),
    onSuccess: (_result, { id }) => invalidateEnvironments(queryClient, id),
  });
}

export function useCancelMigration() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');
  return useMutation({
    mutationFn: (id: string) => api.del<EnvironmentListItem>(`/environments/${id}/migrate`).then(assertSuccess),
    meta: { success: t('migrate.toast.cancelled') },
    onSuccess: (_result, id) => invalidateEnvironments(queryClient, id),
  });
}

export function useMergeEnvironments() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ targetId, sourceId, dryRun }: { targetId: string; sourceId: string; dryRun?: boolean }) =>
      api
        .post<MergeResult>(`/environments/${targetId}/merge${dryRun ? '?dryRun=true' : ''}`, { sourceId })
        .then(assertSuccess),
    onSuccess: (result) => {
      if (!result.dryRun) invalidateEnvironments(queryClient, result.targetId);
    },
  });
}

export function useDuplicateEnvironments(enabled = true) {
  return useQuery({
    queryKey: ['environment-duplicates'],
    enabled,
    queryFn: async () => (await api.get<DuplicateGroup[]>('/environments/duplicates').then(assertSuccess)) ?? [],
    staleTime: 60_000,
  });
}
