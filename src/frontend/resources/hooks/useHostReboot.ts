// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type ApiResponse } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { hostTargetKey, hostTargetPath, hostTargetReady, type HostTarget } from './useHostUpdates';

export type HostRebootStatus = 'scheduled' | 'running' | 'stopping_workloads' | 'rebooting' | 'done' | 'failed';

export const REBOOT_REASONS = [
  'demo_mode',
  'reboot_in_progress',
  'invalid_time',
  'not_confirmed',
  'not_scheduled',
  'host_unreachable',
  'kubernetes_node',
  'swarm_worker',
  'swarm_pause_failed',
  'guest_not_graceful',
  'guest_shutdown_failed',
  'guest_timeout',
  'container_stop_failed',
  'container_timeout',
  'reboot_command_failed',
  'host_not_back',
  'host_did_not_restart',
  'interrupted',
  'restore_partial',
] as const;

export type RebootReason = (typeof REBOOT_REASONS)[number];

export function isRebootReason(value: string | undefined): value is RebootReason {
  return REBOOT_REASONS.some((reason) => reason === value);
}

export type HostReboot = {
  id: string;
  status: HostRebootStatus;
  phase?: string;
  scheduledAt: string;
  drainedExternally: boolean;
  requestedBy?: string;
  startedAt?: string;
  rebootIssuedAt?: string;
  finishedAt?: string;
  failureReason?: string;
  lastError?: string;
};

export type HostRebootState = {
  reboot?: HostReboot;
  blockedReason?: string;
};

export type HostRebootInventory = {
  mcharborHost: boolean;
  containersKnown: boolean;
  containers: number;
  containersReturning: number;
  guestsKnown: boolean;
  guests: number;
  guestBlockers?: string[];
  swarm?: string;
  swarmTasks: number;
  kubernetesKnown: boolean;
  kubernetes?: string[];
};

export type HostRebootRequest = {
  confirm: true;
  now: boolean;
  scheduledAt?: string;
  drainedExternally: boolean;
};

const ACTIVE: HostRebootStatus[] = ['scheduled', 'running', 'stopping_workloads', 'rebooting'];

export function rebootInFlight(currentStatus: HostRebootStatus | undefined): boolean {
  return currentStatus === 'running' || currentStatus === 'stopping_workloads' || currentStatus === 'rebooting';
}

export function useHostRebootState(target: HostTarget, enabled: boolean) {
  return useQuery({
    queryKey: ['host-reboot', hostTargetKey(target)],
    enabled: enabled && hostTargetReady(target),
    retry: false,
    refetchInterval: (query) => {
      const currentStatus = query.state.data?.reboot?.status;
      return currentStatus && ACTIVE.includes(currentStatus) ? 10_000 : false;
    },
    queryFn: () => api.get<HostRebootState>(hostTargetPath(target, '/os-reboot')).then(assertSuccess),
  });
}

export function useHostRebootInventory(target: HostTarget, enabled: boolean) {
  return useQuery({
    queryKey: ['host-reboot-inventory', hostTargetKey(target)],
    enabled: enabled && hostTargetReady(target),
    retry: false,
    staleTime: 30_000,
    queryFn: () => api.get<HostRebootInventory>(hostTargetPath(target, '/os-reboot/inventory')).then(assertSuccess),
  });
}

function useRebootResult() {
  const { t } = useTranslation('environments');
  return <T>(res: ApiResponse<T>): T => {
    if (!res.success) {
      const reason = (res.data as { reason?: string } | undefined)?.reason;
      if (isRebootReason(reason)) {
        throw new Error(t(`detail.hostReboot.reason.${reason}`));
      }
    }
    return assertSuccess(res);
  };
}

export function useScheduleHostReboot(target: HostTarget) {
  const queryClient = useQueryClient();
  const result = useRebootResult();
  return useMutation({
    mutationFn: (request: HostRebootRequest) =>
      api.post<HostReboot>(hostTargetPath(target, '/os-reboot'), request).then(result),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['host-reboot', hostTargetKey(target)] });
    },
  });
}

export function useCancelHostReboot(target: HostTarget) {
  const queryClient = useQueryClient();
  const result = useRebootResult();
  return useMutation({
    mutationFn: () => api.del<{ cancelled: boolean }>(hostTargetPath(target, '/os-reboot')).then(result),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['host-reboot', hostTargetKey(target)] });
    },
  });
}
