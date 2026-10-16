// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type ApiResponse } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';

/** The host interface to create beside a macvlan network; ipv6Auto asks for SLAAC. */
export type HostLinkRequest = {
  name: string;
  ipv4?: string;
  ipv6?: string;
  ipv6Auto?: boolean;
};

type HostLinkState = 'present' | 'missing' | 'unknown' | 'none';

export type HostLinkStatus = {
  hostLink: HostLinkRequest | null;
  parent?: string;
  state: HostLinkState;
  addresses?: string[];
  available: boolean;
  unavailable?: HostLinkUnavailable;
};

export type HostLinkUnavailable = 'agent_unreachable' | 'agent_outdated' | 'containerized';

type Refusals = Record<string, string>;

/**
 * The server names a refused field, or a failed host link step, as a reason on its envelope; without
 * this the operator reads "invalid request body" and has to guess which field it means. A reason that
 * carries the agent's own words keeps them, because those are what the operator can act on.
 */
export function useNetworkRefusals(): (res: ApiResponse<unknown>) => void {
  const { t } = useTranslation('networks');
  const refusals: Refusals = {
    invalid_subnet: t('create.errors.invalidSubnet'),
    invalid_gateway: t('create.errors.invalidGateway'),
    invalid_ip_range: t('create.errors.invalidIpRange'),
    invalid_aux_address: t('create.errors.invalidAuxAddress'),
    gateway_outside_subnet: t('create.errors.gatewayOutsideSubnet'),
    ip_range_outside_subnet: t('create.errors.ipRangeOutsideSubnet'),
    ipv6_subnet_needs_enable: t('create.errors.ipv6SubnetNeedsEnable'),
    address_family_mismatch: t('create.errors.addressFamilyMismatch'),
    invalid_host_link_name: t('hostLink.errors.invalidName'),
    host_link_name_reserved: t('hostLink.errors.nameReserved'),
    host_link_needs_macvlan: t('hostLink.errors.needsMacvlan'),
    host_link_address_invalid: t('hostLink.errors.addressInvalid'),
    host_link_address_in_range: t('hostLink.errors.addressInRange'),
    host_link_passthru: t('hostLink.errors.passthru'),
    host_link_not_configured: t('hostLink.errors.notConfigured'),
    host_link_name_taken: t('hostLink.errors.nameTaken'),
    host_link_failed: t('hostLink.errors.failed'),
    host_link_unavailable: t('hostLink.errors.unavailable'),
    recreate_restored: t('recreate.errors.restored'),
    recreate_lost: t('recreate.errors.lost'),
  };
  const unavailable: Refusals = {
    agent_unreachable: t('hostLink.unavailable.agent_unreachable'),
    agent_outdated: t('hostLink.unavailable.agent_outdated'),
    containerized: t('hostLink.unavailable.containerized'),
  };
  return (res) => {
    const data = res.data as { reason?: unknown; unavailable?: unknown } | undefined;
    const reason = data?.reason;
    if (res.success || typeof reason !== 'string' || !(reason in refusals)) return;
    const why = typeof data?.unavailable === 'string' ? unavailable[data.unavailable] : undefined;
    throw new Error([refusals[reason], why, res.detail].filter(Boolean).join(' '));
  };
}

export function useNetworkHostLink(id: string, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['network-host-link', envId, id],
    queryFn: () =>
      api.get<HostLinkStatus>(`/networks/${id}/host-link`, envId ? { env: envId } : {}).then(assertSuccess),
    enabled: enabled && !!id,
    refetchInterval: 15_000,
  });
}

export function useApplyHostLink(id: string) {
  const { t } = useTranslation('networks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const refuse = useNetworkRefusals();

  return useMutation({
    mutationFn: () =>
      api.post<HostLinkStatus>(`/networks/${id}/host-link/apply${envId ? `?env=${envId}` : ''}`, {}).then((res) => {
        refuse(res);
        return assertSuccess(res);
      }),
    meta: { success: t('hostLink.applied') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['network-host-link'] });
    },
  });
}
