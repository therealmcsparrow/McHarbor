// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { PERM } from '@core/auth/permissionKeys';

export type HostTarget = { kind: 'environment'; envId: string } | { kind: 'self' };

export type HostPermissionNeed = 'servicesView' | 'servicesManage' | 'updatesView' | 'updatesManage' | 'reboot';

/**
 * The key a host control is gated on. McHarbor's own host has its own global
 * keys (host_self.*), so the same component shows the self key there and the
 * per-environment key everywhere else — matching what the backend enforces.
 */
export function hostPermission(target: HostTarget, need: HostPermissionNeed): string {
  if (target.kind === 'self') {
    if (need === 'reboot') return PERM.HOST_SELF_REBOOT;
    return need === 'servicesView' || need === 'updatesView' ? PERM.HOST_SELF_VIEW : PERM.HOST_SELF_MANAGE;
  }
  switch (need) {
    case 'servicesView':
      return PERM.HOST_SERVICES_VIEW;
    case 'servicesManage':
      return PERM.HOST_SERVICES_MANAGE;
    case 'updatesView':
      return PERM.HOST_UPDATES_VIEW;
    case 'updatesManage':
      return PERM.HOST_UPDATES_MANAGE;
    case 'reboot':
      return PERM.HOST_UPDATES_REBOOT;
  }
}

export function hostTargetKey(target: HostTarget): string {
  return target.kind === 'self' ? 'self' : target.envId;
}

export function hostTargetPath(target: HostTarget, path: string): string {
  if (target.kind === 'self') return `/system/self${path}`;
  const joiner = path.includes('?') ? '&' : '?';
  return `/system${path}${joiner}env=${encodeURIComponent(target.envId)}`;
}

export function hostTargetReady(target: HostTarget): boolean {
  return target.kind === 'self' || Boolean(target.envId);
}

export type HostOSInfo = {
  prettyName?: string;
  distro?: string;
  version?: string;
  kernel?: string;
  arch?: string;
  hostname?: string;
  uptimeSeconds?: number;
  manager?: string;
  virtualization?: string;
  rebootRequired: boolean;
  unsupported?: string;
};

export type HostUpdateSchedule = {
  enabled: boolean;
  time: string;
  days: string[];
  withNewPackages: boolean;
  skipUnavailable: boolean;
  removeUnused: boolean;
};

export type HostUpdateApplyOptions = {
  withNewPackages: boolean;
  skipUnavailable: boolean;
  removeUnused: boolean;
};

export type HostUpdateState = {
  schedule: HostUpdateSchedule;
  available: boolean;
  count: number;
  manager?: string;
  updates?: string[];
  lastCheckedAt?: string;
  runningJobId?: string;
  recoveryUnsupported?: boolean;
};

export type HostUpdateJobStatus = {
  jobId: string;
  lines: string[];
  offset: number;
  done: boolean;
  success: boolean;
  error?: string;
  hint?: string;
  startedAt: string;
};

export function useHostOSInfo(target: HostTarget, enabled: boolean) {
  return useQuery({
    queryKey: ['host-os-info', hostTargetKey(target)],
    enabled: enabled && hostTargetReady(target),
    staleTime: 60_000,
    retry: false,
    queryFn: () => api.get<HostOSInfo>(hostTargetPath(target, '/os-info')).then(assertSuccess),
  });
}

export function useHostUpdateState(target: HostTarget, enabled: boolean) {
  return useQuery({
    queryKey: ['host-update-state', hostTargetKey(target)],
    enabled: enabled && hostTargetReady(target),
    retry: false,
    queryFn: () => api.get<HostUpdateState>(hostTargetPath(target, '/os-updates/state')).then(assertSuccess),
  });
}

export function useHostUpdateCheck(target: HostTarget) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.get(hostTargetPath(target, '/os-updates/check')).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ['host-update-state', hostTargetKey(target)],
      });
    },
  });
}

export function useSaveHostUpdateSchedule(target: HostTarget) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (schedule: HostUpdateSchedule) =>
      api.put(hostTargetPath(target, '/os-updates/schedule'), schedule).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ['host-update-state', hostTargetKey(target)],
      });
    },
  });
}

export function useStartHostUpdate(target: HostTarget) {
  return useMutation({
    mutationFn: (options: HostUpdateApplyOptions) =>
      api
        .post<HostUpdateJobStatus>(hostTargetPath(target, '/os-updates/apply'), {
          confirm: true,
          withNewPackages: options.withNewPackages,
          skipUnavailable: options.skipUnavailable,
          removeUnused: options.removeUnused,
        })
        .then(assertSuccess),
  });
}

export type HostUpdateProgress = {
  lines: string[];
  running: boolean;
  done: boolean;
  success: boolean;
  error: string;
  hint: string;
};

const IDLE_PROGRESS: HostUpdateProgress = {
  lines: [],
  running: false,
  done: false,
  success: false,
  error: '',
  hint: '',
};

export function useHostUpdateProgress(target: HostTarget, jobId: string | null) {
  const [progress, setProgress] = useState<HostUpdateProgress>(IDLE_PROGRESS);
  const sourceRef = useRef<EventSource | null>(null);
  const queryClient = useQueryClient();
  const key = hostTargetKey(target);
  const streamPath = hostTargetReady(target) ? hostTargetPath(target, '/os-updates/stream') : '';

  const reset = useCallback(() => setProgress(IDLE_PROGRESS), []);

  const jobKey = jobId && streamPath ? `${jobId}|${streamPath}` : '';
  const [appliedJobKey, setAppliedJobKey] = useState('');
  if (appliedJobKey !== jobKey) {
    setAppliedJobKey(jobKey);
    if (jobKey) setProgress({ ...IDLE_PROGRESS, running: true });
  }

  useEffect(() => {
    if (!jobId || !streamPath) return;

    const joiner = streamPath.includes('?') ? '&' : '?';
    const source = new EventSource(`/api${streamPath}${joiner}job=${encodeURIComponent(jobId)}`);
    sourceRef.current = source;

    source.onmessage = (evt) => {
      if (sourceRef.current !== source) return;
      const currentStatus = JSON.parse(evt.data) as HostUpdateJobStatus;
      setProgress((prev) => ({
        lines: [...prev.lines, ...(currentStatus.lines ?? [])],
        running: !currentStatus.done,
        done: currentStatus.done,
        success: currentStatus.success,
        error: currentStatus.error ?? '',
        hint: currentStatus.hint ?? '',
      }));
      if (currentStatus.done) {
        source.close();
        void queryClient.invalidateQueries({
          queryKey: ['host-update-state', key],
        });
        void queryClient.invalidateQueries({
          queryKey: ['host-os-info', key],
        });
      }
    };

    source.onerror = () => {
      if (sourceRef.current !== source) return;
      source.close();
      setProgress((prev) => (prev.done ? prev : { ...prev, running: false }));
    };

    return () => {
      sourceRef.current = null;
      source.close();
    };
  }, [key, streamPath, jobId, queryClient]);

  return { progress, reset };
}
