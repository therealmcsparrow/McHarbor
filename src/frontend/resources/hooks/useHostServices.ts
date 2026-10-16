// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { hostTargetKey, type HostTarget } from './useHostUpdates';

import type {
  HostService,
  HostServiceList,
  HostServiceAction,
  HostServiceActionResult,
  HostServiceOverride,
  HostServiceOverrideInput,
  HostServiceOverrideResult,
} from './hostServiceTypes';
export type * from './hostServiceTypes';

export const hostServicesKey = (target: HostTarget) => ['host-services', hostTargetKey(target)] as const;

const basePath = (target: HostTarget) => (target.kind === 'self' ? '/host-services/self' : '/host-services');

const unitPath = (target: HostTarget, unit: string) => `${basePath(target)}/${encodeURIComponent(unit)}`;

const scopeParams = (target: HostTarget): Record<string, string> | undefined =>
  target.kind === 'self' ? undefined : { env: target.envId };

const withScope = (path: string, target: HostTarget) =>
  target.kind === 'self' ? path : `${path}?env=${encodeURIComponent(target.envId)}`;

const targetReady = (target: HostTarget) => target.kind === 'self' || Boolean(target.envId);

export function isHostServiceRunning(unit: HostService): boolean {
  return unit.activeState === 'active' || unit.activeState === 'reloading' || unit.activeState === 'activating';
}

export function isHostServiceEnabled(unit: HostService): boolean {
  return unit.unitFileState === 'enabled' || unit.unitFileState === 'enabled-runtime' || unit.unitFileState === 'alias';
}

export function useHostServices(target: HostTarget) {
  return useQuery({
    queryKey: hostServicesKey(target),
    enabled: targetReady(target),
    retry: false,
    queryFn: () => api.get<HostServiceList>(basePath(target), scopeParams(target)).then(assertSuccess),
  });
}

export function useHostServiceOverride(target: HostTarget, unit: string | null) {
  return useQuery({
    queryKey: [...hostServicesKey(target), 'override', unit],
    enabled: targetReady(target) && Boolean(unit),
    retry: false,
    gcTime: 0,
    queryFn: () =>
      api.get<HostServiceOverride>(`${unitPath(target, unit ?? '')}/override`, scopeParams(target)).then(assertSuccess),
  });
}

function useHostServiceMutation<TVariables, TData>(
  target: HostTarget,
  request: (variables: TVariables) => Promise<TData>,
  success: (data: TData, variables: TVariables) => string,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: request,
    meta: {
      success: (data: unknown, variables: unknown) => success(data as TData, variables as TVariables),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: hostServicesKey(target),
      });
    },
  });
}

export function useHostServiceAction(target: HostTarget) {
  const { t } = useTranslation('environments');
  return useHostServiceMutation(
    target,
    ({ unit, action }: { unit: string; action: HostServiceAction }) =>
      api
        .post<HostServiceActionResult>(withScope(`${unitPath(target, unit)}/${action}`, target), {})
        .then(assertSuccess),
    (data, { unit, action }) => {
      if (data?.warning === 'agent_restart') {
        return t('hostServices.toast.agentRestarting');
      }
      if (data?.warning === 'mcharbor_restart') {
        return t('hostServices.toast.mcharborRestarting', { unit });
      }
      return t(`hostServices.toast.${action}`, { unit });
    },
  );
}

export function useSaveHostServiceOverride(target: HostTarget) {
  const { t } = useTranslation('environments');
  return useHostServiceMutation(
    target,
    ({ unit, content, reload, verify }: HostServiceOverrideInput) =>
      api
        .put<HostServiceOverrideResult>(withScope(`${unitPath(target, unit)}/override`, target), {
          content,
          reload,
          verify,
        })
        .then(assertSuccess),
    (_data, { unit }) => t('hostServices.toast.overrideSaved', { unit }),
  );
}

export function useRevertHostServiceOverride(target: HostTarget) {
  const { t } = useTranslation('environments');
  return useHostServiceMutation(
    target,
    (unit: string) => api.del<unknown>(`${unitPath(target, unit)}/override`, scopeParams(target)).then(assertSuccess),
    (_data, unit) => t('hostServices.toast.overrideReverted', { unit }),
  );
}

export function useHostDaemonReload(target: HostTarget) {
  const { t } = useTranslation('environments');
  return useHostServiceMutation(
    target,
    (_: void) => api.post<unknown>(withScope(`${basePath(target)}/daemon-reload`, target), {}).then(assertSuccess),
    () => t('hostServices.toast.reloaded'),
  );
}
