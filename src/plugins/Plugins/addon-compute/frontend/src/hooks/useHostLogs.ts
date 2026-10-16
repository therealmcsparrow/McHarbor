// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { HostLogResult, HostLogSource, HostLogTarget, LifecycleEvent, LifecycleListResult } from '../types';

const OS_SOURCES: HostLogSource[] = ['system', 'kernel', 'auth', 'docker', 'libvirt', 'agent', 'qemu'];

/** How often Live re-reads a host log; each read is a shell call on the host. */
export const HOST_LOG_POLL_MS = 5_000;

export interface HostLogView {
  lines: string[];
  notices: string[];
  availableFiles: string[];
  /** Guests a qemu read can name, from the host's own guest units, or its libvirt logs on an older agent. */
  guests: HostLogTarget[];
  isLoading: boolean;
  isFetching: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
}

function activityLine(lifecycleEvent: LifecycleEvent): string {
  const at = lifecycleEvent.timestamp.replace('T', ' ').replace(/\.\d+/, '').replace('Z', '');
  const subject = lifecycleEvent.subjectName || lifecycleEvent.subjectId;
  const state = lifecycleEvent.state ? ` (${lifecycleEvent.state})` : '';
  return `${at} ${lifecycleEvent.severity.toUpperCase()} ${lifecycleEvent.eventType}/${lifecycleEvent.action} ${lifecycleEvent.subjectType} ${subject}${state}`;
}

export function useHostLogs(
  envId: string,
  source: HostLogSource,
  tail: number,
  target: string,
  live = false,
): HostLogView {
  const isOs = OS_SOURCES.includes(source);

  const activity = useQuery({
    queryKey: ['compute', 'host-activity', envId, tail],
    queryFn: async () => {
      const response = await api.get<LifecycleListResult>('/lifecycle', {
        envId,
        page: '1',
        perPage: String(tail),
      });
      return response.data?.items ?? [];
    },
    enabled: Boolean(envId) && source === 'activity',
    refetchOnWindowFocus: false,
    refetchInterval: live ? HOST_LOG_POLL_MS : false,
  });

  const os = useQuery({
    queryKey: ['compute', 'host-os-logs', envId, source, tail, target],
    queryFn: async () => {
      const response = await api.get<HostLogResult>('/system/os-logs', {
        env: envId,
        source,
        tail: String(tail),
        ...(target ? { target } : {}),
      });
      return assertSuccess(response);
    },
    // A qemu read without a guest only lists guests, which the guests query already does.
    enabled: Boolean(envId) && isOs && (source !== 'qemu' || target !== ''),
    refetchOnWindowFocus: false,
    retry: false,
    staleTime: 30_000,
    refetchInterval: live ? HOST_LOG_POLL_MS : false,
  });

  // Kept apart from the read itself: once a guest is picked, that response no longer lists the others.
  const guestList = useQuery({
    queryKey: ['compute', 'host-log-guests', envId],
    queryFn: async () => {
      const result = assertSuccess(
        await api.get<HostLogResult>('/system/os-logs', { env: envId, source: 'qemu', tail: '1' }),
      );
      const units = result?.targets ?? [];
      const files = (result?.availableFiles ?? []).map((file) => {
        const id = file.replace(/\.log$/, '');
        return { id, label: id };
      });
      return { guests: [...units, ...files], notices: result?.notices ?? [] };
    },
    enabled: Boolean(envId) && source === 'qemu',
    refetchOnWindowFocus: false,
    retry: false,
    staleTime: 30_000,
  });
  const guests = guestList.data?.guests ?? [];

  if (source === 'activity') {
    return {
      lines: (activity.data ?? []).map(activityLine),
      notices: [],
      availableFiles: [],
      guests: [],
      isLoading: activity.isLoading,
      isFetching: activity.isFetching,
      isError: activity.isError,
      error: activity.error,
      refetch: () => void activity.refetch(),
    };
  }

  return {
    lines: os.data?.lines ?? [],
    notices: source === 'qemu' && target === '' ? (guestList.data?.notices ?? []) : (os.data?.notices ?? []),
    availableFiles: os.data?.availableFiles ?? [],
    guests,
    isLoading: os.isLoading || guestList.isLoading,
    isFetching: os.isFetching || guestList.isFetching,
    isError: os.isError || guestList.isError,
    error: os.error ?? guestList.error,
    refetch: () => {
      if (source === 'qemu') void guestList.refetch();
      void os.refetch();
    },
  };
}
