// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type HARole = 'peer' | 'active' | 'standby';
export type NodeState = 'running' | 'draining' | 'stopped';

export type ClusterInstance = {
  id: string;
  hostname: string;
  version: string;
  build: string;
  haRole: HARole;
  state: NodeState;
  advertiseUrl: string;
  agents: number;
  startedAt: string;
  heartbeatAt: string;
  alive: boolean;
  self: boolean;
};

export type ReadyCheck = { name: string; ok: boolean };

export type ClusterStatus = {
  nodeId: string;
  databaseDriver: string;
  roles: Record<string, boolean>;
  database: { reachable: boolean };
  serverTime: string;
  instances: ClusterInstance[];
  activeConflict?: string[];
  role: HARole;
  state: NodeState;
  mode: 'peer' | 'active-passive' | 'active-active';
  ready: boolean;
  checks: ReadyCheck[];
  pluginStore: 'local' | 'database' | 'shared-fs' | 'object';
};

export function useClusterStatus(enabled = true) {
  return useQuery({
    queryKey: ['cluster', 'status'],
    enabled,
    queryFn: () => api.get<ClusterStatus>('/cluster/status').then(assertSuccess),
    refetchInterval: 5_000,
  });
}

function useClusterAction<T>(send: (arg: T) => Promise<unknown>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: send,
    onSettled: () => void client.invalidateQueries({ queryKey: ['cluster'] }),
  });
}

export const useFailover = () =>
  useClusterAction((targetNodeId: string) => api.post('/cluster/failover', { targetNodeId }).then(assertSuccess));

export const useDrain = () =>
  useClusterAction(({ nodeId, drain }: { nodeId: string; drain: boolean }) =>
    api.post(`/cluster/instances/${encodeURIComponent(nodeId)}/${drain ? 'drain' : 'resume'}`).then(assertSuccess),
  );

export function useNodeRole() {
  return useQuery({
    queryKey: ['cluster', 'health-role'],
    queryFn: () => api.get<{ role?: HARole; nodeId?: string }>('/health').then(assertSuccess),
    refetchInterval: 30_000,
    retry: false,
  });
}

export type ReadinessTarget = 'active-active' | 'active-passive';
export type GateStatus = 'pass' | 'warn' | 'fail';

export type ReadinessGate = {
  id: string;
  status: GateStatus;
  detail: string;
  remediation?: string;
  values?: string[];
};

export type ClusterReadiness = {
  target: ReadinessTarget;
  ready: boolean;
  enabled: boolean;
  gates: ReadinessGate[];
};

export type StartupHealth = {
  nodeId: string;
  checkedAt: string;
  databaseDriver: string;
  databaseReachable: boolean;
  dataDirWritable: boolean;
  dataDirFreeBytes: number;
  lowDisk: boolean;
  encryptionKeySource: 'env' | 'file';
  tlsMode: 'off' | 'https' | 'https-only';
  pluginStore: string;
  haMode: string;
  haModeRefused: boolean;
};

export function useClusterReadiness(target: ReadinessTarget, enabled = true) {
  return useQuery({
    queryKey: ['cluster', 'readiness', target],
    enabled,
    queryFn: () =>
      api.get<ClusterReadiness>(`/cluster/readiness?target=${encodeURIComponent(target)}`).then(assertSuccess),
    refetchInterval: 30_000,
  });
}

export type AgentOwner = { envId: string; nodeId: string; advertiseUrl: string; expiresAt: string };
export type RoutingProofStatus = 'unproven' | 'proven' | 'failed';
export type RoutingProof = {
  status: RoutingProofStatus;
  nodeId?: string;
  envId?: string;
  at?: string;
  error?: string;
};
export type RoutingProbe = {
  envId: string;
  nodeId: string;
  ok: boolean;
  status?: number;
  error?: string;
  duration?: string;
};
export type AgentRouting = {
  nodeId: string;
  canForward: boolean;
  owners: AgentOwner[];
  proof: RoutingProof;
  probes?: RoutingProbe[];
};

export function useAgentRouting(enabled = true) {
  return useQuery({
    queryKey: ['cluster', 'agent-routing'],
    enabled,
    queryFn: () => api.get<AgentRouting>('/cluster/agent-routing').then(assertSuccess),
    refetchInterval: 30_000,
  });
}

export const useAgentRoutingCheck = () =>
  useClusterAction(() => api.post<AgentRouting>('/cluster/agent-routing-check').then(assertSuccess));

export function useStartupHealth(enabled = true) {
  return useQuery({
    queryKey: ['cluster', 'startup-health'],
    enabled,
    queryFn: () => api.get<StartupHealth>('/cluster/startup-health').then(assertSuccess),
  });
}
