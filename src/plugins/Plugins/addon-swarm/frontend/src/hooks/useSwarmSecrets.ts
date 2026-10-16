// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '../host-env';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { SwarmConfig, SwarmPayloadWriteRequest, SwarmSecret } from '../types';

function envParams(envId: string | null | undefined): Record<string, string> {
  return envId ? { env: envId } : {};
}

function envSuffix(envId: string | null | undefined): string {
  return envId ? `?env=${envId}` : '';
}

// --- secrets ---------------------------------------------------------------

/**
 * Swarm secret metadata.
 *
 * There is no companion hook that reads a secret's value, and there is no
 * endpoint that would answer one: Docker's secret API is write-only for the
 * payload. Any UI that appears to offer a reveal here would be lying.
 */
export function useSwarmSecrets(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'secrets', envId],
    queryFn: () => api.get<SwarmSecret[]>('/swarm/secrets', envParams(envId)).then((r) => assertSuccess(r) ?? []),
    enabled,
    refetchInterval: 60_000,
  });
}

export function useCreateSwarmSecret() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: SwarmPayloadWriteRequest) =>
      api.post<{ id: string }>(`/swarm/secrets${envSuffix(envId)}`, body).then(assertSuccess),
    meta: { success: t('toast.secretCreated') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['swarm', 'secrets'] }),
  });
}

export function useRemoveSwarmSecret() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.del(`/swarm/secrets/${id}`, envParams(envId)).then(assertSuccess),
    meta: { success: t('toast.secretRemoved') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['swarm', 'secrets'] }),
  });
}

// --- configs ---------------------------------------------------------------

export function useSwarmConfigs(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'configs', envId],
    queryFn: () => api.get<SwarmConfig[]>('/swarm/configs', envParams(envId)).then((r) => assertSuccess(r) ?? []),
    enabled,
    refetchInterval: 60_000,
  });
}

/**
 * One config, payload included.
 *
 * Enabled only while the viewer dialog is open — the payload is the one thing
 * in this module that is both readable and worth not scattering through the
 * query cache of every page that lists configs.
 */
export function useSwarmConfig(id: string | undefined, enabled: boolean) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['swarm', 'config', id, envId],
    queryFn: () => api.get<SwarmConfig>(`/swarm/configs/${id}`, envParams(envId)).then(assertSuccess),
    enabled: Boolean(id) && enabled,
    gcTime: 0,
  });
}

export function useCreateSwarmConfig() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: SwarmPayloadWriteRequest) =>
      api.post<{ id: string }>(`/swarm/configs${envSuffix(envId)}`, body).then(assertSuccess),
    meta: { success: t('toast.configCreated') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['swarm', 'configs'] }),
  });
}

export function useRemoveSwarmConfig() {
  const { t } = useTranslation('swarm');
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.del(`/swarm/configs/${id}`, envParams(envId)).then(assertSuccess),
    meta: { success: t('toast.configRemoved') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['swarm', 'configs'] }),
  });
}
