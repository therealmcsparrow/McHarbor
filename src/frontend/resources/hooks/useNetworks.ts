// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { NetworkInfo, ContainerInfo } from '@core/types/docker';
import { useNetworkRefusals, type HostLinkRequest, type HostLinkUnavailable } from './networkHostLink';

export type IPAMConfig = {
  Subnet?: string;
  Gateway?: string;
  IPRange?: string;
  AuxiliaryAddresses?: Record<string, string>;
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
  enableIPv6?: boolean;
  ipam?: IPAMOptions;
  options?: Record<string, string>;
  labels?: Record<string, string>;
  hostLink?: HostLinkRequest;
};

export type HostInterface = {
  name: string;
  kind?: string;
  source: 'agent' | 'in-use';
  up: boolean;
  mac?: string;
  mtu?: number;
  addresses?: string[];
  inUseBy?: string[];
  hasDefaultRoute?: boolean;
};

export type HostInterfaces = {
  interfaces: HostInterface[];
  agentAvailable: boolean;
  /** Why nothing listed the host's links when agentAvailable is false. */
  unavailable?: 'agent_unreachable' | 'containerized';
  hostLinkAvailable?: boolean;
  /** Why a host interface cannot be created here when hostLinkAvailable is false. */
  hostLinkUnavailable?: HostLinkUnavailable;
};

export function useHostInterfaces(enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['network-host-interfaces', envId],
    queryFn: () =>
      api
        .get<HostInterfaces>('/networks/host-interfaces', envId ? { env: envId } : {})
        .then((r) => r.data ?? { interfaces: [], agentAvailable: false }),
    staleTime: 60_000,
    enabled,
  });
}

export function useNetworks() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['networks', envId],
    queryFn: () => api.get<NetworkInfo[]>('/networks', envId ? { env: envId } : {}).then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
}

export function useCreateNetwork() {
  const { t } = useTranslation('networks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const refuse = useNetworkRefusals();

  return useMutation({
    mutationFn: (params: CreateNetworkRequest) =>
      api.post(`/networks${envId ? `?env=${envId}` : ''}`, params).then((res) => {
        refuse(res);
        return assertSuccess(res);
      }),
    meta: { success: t('toast.created') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}

export function useRemoveNetwork() {
  const { t } = useTranslation('networks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: (id: string) => api.del(`/networks/${id}`, envId ? { env: envId } : {}).then(assertSuccess),
    meta: { success: t('toast.removed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}

export function useNetworkInspect(id: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['network', envId, id],
    queryFn: () =>
      api.get<NetworkInfo>(`/networks/${id}`, envId ? { env: envId } : {}).then((r) => {
        if (!r.data) {
          throw new Error('network response missing data');
        }
        return r.data;
      }),
    enabled: !!id,
    refetchInterval: 15_000,
  });
}

export function useContainersForNetwork() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['containers', envId, true],
    queryFn: () => api.get<ContainerInfo[]>('/containers', { env: envId ?? '', all: 'true' }).then((r) => r.data ?? []),
  });
}

export function useConnectNetwork() {
  const { t } = useTranslation('networks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: ({ networkId, container }: { networkId: string; container: string }) =>
      api.post(`/networks/${networkId}/connect${envId ? `?env=${envId}` : ''}`, { container }).then(assertSuccess),
    meta: { success: t('toast.connected') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['network'] });
      void queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}

export function useDisconnectNetwork() {
  const { t } = useTranslation('networks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: ({ networkId, container }: { networkId: string; container: string }) =>
      api.post(`/networks/${networkId}/disconnect${envId ? `?env=${envId}` : ''}`, { container }).then(assertSuccess),
    meta: { success: t('toast.disconnected') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['network'] });
      void queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}
