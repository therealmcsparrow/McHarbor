// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type AgentProgress = {
  status: string;
  mode?: string;
  error?: string;
  target?: string;
};

export type AgentRow = {
  envId: string;
  name: string;
  hostname?: string;
  installKind?: string;
  remoteIp?: string;
  version?: string;
  build?: string;
  os?: string;
  arch?: string;
  outdated: boolean;
  updatable: boolean;
  targetVersion?: string;
  targetBuild?: string;
  progress?: AgentProgress;
};

export type AgentInventory = {
  agents: AgentRow[];
  targetVersion: string;
  targetBuild?: string;
  updating?: boolean;
  excludedEnvIds?: string[];
};

export type AgentState = 'updating' | 'pending' | 'started' | 'failed' | 'tooOld' | 'excluded' | 'outdated' | 'current';

const IN_FLIGHT = new Set(['pending', 'updating']);
const PROGRESS_STATES = new Set(['updating', 'pending', 'started', 'failed']);

export const AGENT_INVENTORY_KEY = ['updates', 'agents'] as const;

export function inFlight(agent: AgentRow): boolean {
  return agent.progress ? IN_FLIGHT.has(agent.progress.status) : false;
}

export function agentState(agent: AgentRow, excluded: boolean): AgentState {
  const progressStatus = agent.progress?.status;
  if (progressStatus && PROGRESS_STATES.has(progressStatus)) return progressStatus as AgentState;
  if (!agent.updatable) return 'tooOld';
  if (excluded) return 'excluded';
  if (agent.outdated) return 'outdated';
  return 'current';
}

export function agentPlatform(agent: AgentRow): string {
  return [agent.os, agent.arch].filter(Boolean).join('/');
}

export function useAgentInventory() {
  return useQuery({
    queryKey: AGENT_INVENTORY_KEY,
    queryFn: () => api.get<AgentInventory>('/updates/agents').then(assertSuccess),
    refetchInterval: (query) => (query.state.data?.updating || query.state.data?.agents.some(inFlight) ? 1500 : 15000),
    refetchIntervalInBackground: false,
  });
}

export function useUpdateAgents() {
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (envIds: string[]) => api.post<unknown>('/updates/agents/update', { envIds }).then(assertSuccess),
    meta: { success: t('about.autoUpdate.agentsUpdateStarted') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: AGENT_INVENTORY_KEY }),
  });
}
