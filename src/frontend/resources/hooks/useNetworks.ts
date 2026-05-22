// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { NetworkInfo, ContainerInfo } from '@core/types/docker';

export type IPAMConfig = {
  Subnet?: string;
  Gateway?: string;
  IPRange?: string;
};

export type IPAMOptions = {
  Driver?: string;
  Config?: IPAMConfig[];
};

export type CreateNetworkRequest = {
  name: string;
  driver?: string;
  internal?: boolean;
  attachable?: boolean;
  ipam?: IPAMOptions;
  options?: Record<string, string>;
  labels?: Record<string, string>;
};

export function useNetworks() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['networks', envId],
    queryFn: () =>
      api
        .get<NetworkInfo[]>('/networks', envId ? { env: envId } : {})
        .then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
}

export function useCreateNetwork() {
  const { t } = useTranslation('networks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: (params: CreateNetworkRequest) =>
      api.post(`/networks${envId ? `?env=${envId}` : ''}`, params).then(assertSuccess),
    meta: { success: t('toast.created') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}

export function useRemoveNetwork() {
  const { t } = useTranslation('networks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/networks/${id}`, envId ? { env: envId } : {}).then(assertSuccess),
    meta: { success: t('toast.removed') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}

export function useNetworkInspect(id: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['network', envId, id],
    queryFn: () =>
      api
        .get<NetworkInfo>(`/networks/${id}`, envId ? { env: envId } : {})
        .then((r) => r.data!),
    enabled: !!id,
    refetchInterval: 15_000,
  });
}

export function useContainersForNetwork() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['containers', envId, true],
    queryFn: () =>
      api
        .get<ContainerInfo[]>('/containers', { env: envId ?? '', all: 'true' })
        .then((r) => r.data ?? []),
  });
}

export function useConnectNetwork() {
  const { t } = useTranslation('networks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: ({ networkId, container }: { networkId: string; container: string }) =>
      api
        .post(`/networks/${networkId}/connect${envId ? `?env=${envId}` : ''}`, { container })
        .then(assertSuccess),
    meta: { success: t('toast.connected') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['network'] });
      queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}

export function useDisconnectNetwork() {
  const { t } = useTranslation('networks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: ({ networkId, container }: { networkId: string; container: string }) =>
      api
        .post(`/networks/${networkId}/disconnect${envId ? `?env=${envId}` : ''}`, { container })
        .then(assertSuccess),
    meta: { success: t('toast.disconnected') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['network'] });
      queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}
