// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { type ModelMeta, refusalOf } from '@resources/hooks/useIncidents';

export type ModelInfo = { provider: string; connection: string; model: string; baseUrl?: string; local: boolean };

export type NarrationSettings = {
  envId: string;
  narrateOptIn: boolean;
  proactive: boolean;
  hourlyCap: number;
  optedInBy?: string;
  optedInAt?: string;
  modelConfigured: boolean;
  model?: ModelInfo;
};

export type NarrationSettingsInput = Pick<NarrationSettings, 'envId' | 'narrateOptIn' | 'proactive' | 'hourlyCap'>;

export type FindingKind =
  | 'memory_climb'
  | 'host_memory_forecast'
  | 'scheduled_restarts'
  | 'post_deploy_failures'
  | 'host_disk_forecast'
  | 'storage_location_forecast';

export type Finding = {
  id: string;
  envId: string;
  subjectType: string;
  subjectName: string;
  kind: FindingKind;
  status: 'open' | 'dismissed' | 'cleared';
  metrics: Record<string, number>;
  firstSeenAt: string;
  lastSeenAt: string;
  explanation?: string;
  explanationMeta?: ModelMeta;
};

export function useNarrationSettings(envId: string | null | undefined) {
  return useQuery({
    queryKey: ['incident-settings', envId],
    enabled: Boolean(envId),
    queryFn: () => api.get<NarrationSettings>('/incidents/settings', { env: envId ?? '' }).then(assertSuccess),
  });
}

export function useSaveNarrationSettings() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: NarrationSettingsInput) =>
      api.put<NarrationSettings>('/incidents/settings', input).then(refusalOf),
    onSuccess: (data) => client.setQueryData(['incident-settings', data.envId], data),
  });
}

export function useFindings(envId: string | null | undefined, statusValue: Finding['status'] | '') {
  return useQuery({
    queryKey: ['incident-findings', envId, statusValue],
    queryFn: () =>
      api
        .get<Finding[]>('/incidents/findings', {
          ...(envId ? { env: envId } : {}),
          ...(statusValue ? { status: statusValue } : {}),
        })
        .then(assertSuccess),
    refetchInterval: 60_000,
  });
}

function useFindingAction(path: 'explain' | 'dismiss') {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<Finding>(`/incidents/findings/${encodeURIComponent(id)}/${path}`).then(refusalOf),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['incident-findings'] }),
  });
}

export const useExplainFinding = () => useFindingAction('explain');
export const useDismissFinding = () => useFindingAction('dismiss');
