// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useRef, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import { createClientId } from '@resources/utils/id';
import type { ContainerInspect } from '@core/types/docker';
export { useContainersBulkStats } from '@resources/hooks/useContainersBulkStats';
export { useContainers } from '@resources/hooks/useContainers';

export type MoveContainerPlan = {
  sourceEnvId: string;
  targetEnvId: string;
  containerId: string;
  containerName: string;
  targetName: string;
  image: {
    reference: string;
    id: string;
    size?: number;
    exists: boolean;
    willTransfer: boolean;
  };
  stack: {
    name?: string;
    service?: string;
    labelsPreserve: boolean;
    managedRecord: boolean;
  };
  volumes: Array<{
    type: string;
    name?: string;
    source?: string;
    destination: string;
    mode?: string;
    exists: boolean;
    willCreate: boolean;
    willCopy: boolean;
    manual: boolean;
  }>;
  networkMode?: string;
  networks: Array<{
    name: string;
    sourceName: string;
    targetName: string;
    id?: string;
    driver?: string;
    exists: boolean;
    willCreate: boolean;
    aliases?: string[];
    targetAliases?: string[];
    ipAddress?: string;
    targetIpAddress?: string;
    macAddress?: string;
    targetMacAddress?: string;
    builtin: boolean;
    internal: boolean;
    attachable: boolean;
    ipam?: {
      Driver?: string;
      Config?: Array<{ Subnet?: string; Gateway?: string; IPRange?: string }>;
    };
    options?: Record<string, string>;
    labels?: Record<string, string>;
  }>;
  ports: Array<{
    containerPort: string;
    hostIp?: string;
    hostPort?: string;
  }>;
  requiredChanges: string[];
  warnings: string[];
};

export type MoveNetworkConfig = {
  sourceName: string;
  targetName: string;
  driver?: string;
  internal?: boolean;
  attachable?: boolean;
  ipam?: {
    Driver?: string;
    Config?: Array<{ Subnet?: string; Gateway?: string; IPRange?: string }>;
  };
  options?: Record<string, string>;
  labels?: Record<string, string>;
  aliases?: string[];
  ipAddress?: string;
  macAddress?: string;
};

export type MoveContainerOptions = {
  id: string;
  targetEnvId: string;
  targetName?: string;
  networkMode?: string;
  networks?: MoveNetworkConfig[];
  transferImage: boolean;
  createMissingNetworks: boolean;
  createMissingVolumes: boolean;
  copyNamedVolumes: boolean;
  startTarget: boolean;
  stopSource: boolean;
  removeSource: boolean;
};

export type MoveContainerEvent = {
  step: number;
  total: number;
  message: string;
  status: 'progress' | 'done' | 'error';
  phase?: string;
  bytesTransferred?: number;
  bytesTotal?: number;
  targetContainerId?: string;
  targetName?: string;
};

export type MoveProgressLogEntry = {
  id: string;
  message: string;
  phase?: string;
};

export function useContainer(id: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container', envId, id],
    queryFn: () =>
      api
        .get<ContainerInspect>(`/containers/${id}`, envId ? { env: envId } : {})
        .then((r) => r.data),
    enabled: !!id,
  });
}

export function useContainerAction() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: string }) =>
      api.post(`/containers/${id}/${action}${envId ? `?env=${envId}` : ''}`).then(assertSuccess),
    meta: {
      success: (_: unknown, vars: unknown) => {
        const { action } = vars as { action: string };
        const labels: Record<string, string> = {
          start: t('toast.started'), stop: t('toast.stopped'), restart: t('toast.restarted'),
          pause: t('toast.paused'), unpause: t('toast.resumed'), kill: t('toast.killed'),
        };
        return labels[action] ?? t('actions.actionCompleted');
      },
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['containers'] });
      queryClient.invalidateQueries({ queryKey: ['container'] });
    },
  });
}

export function useRenameContainer() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => {
      const envQuery = envId ? `?env=${envId}` : '';
      return api.post(`/containers/${id}/rename${envQuery}`, { name }).then(assertSuccess);
    },
    meta: { success: t('toast.renamed') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['containers'] });
      queryClient.invalidateQueries({ queryKey: ['container'] });
    },
  });
}

export function useMoveContainerPlan(
  id: string,
  targetEnvId: string,
  targetName: string,
  networkMode: string,
  networks: MoveNetworkConfig[],
  enabled: boolean,
) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-move-plan', envId, id, targetEnvId, targetName, networkMode, networks],
    queryFn: () => {
      const envQuery = envId ? `?env=${envId}` : '';
      return api
        .post<MoveContainerPlan>(`/containers/${id}/move/plan${envQuery}`, { targetEnvId, targetName, networkMode, networks })
        .then(assertSuccess);
    },
    placeholderData: (previous) => previous,
    enabled: enabled && !!id && !!targetEnvId,
  });
}

export function useMoveContainer() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, ...body }: MoveContainerOptions) => {
      const envQuery = envId ? `?env=${envId}` : '';
      return api.post(`/containers/${id}/move${envQuery}`, body).then(assertSuccess);
    },
    meta: { success: t('toast.moved') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['containers'] });
      queryClient.invalidateQueries({ queryKey: ['container'] });
      queryClient.invalidateQueries({ queryKey: ['images'] });
      queryClient.invalidateQueries({ queryKey: ['volumes'] });
      queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}

function createMoveLogEntry(message: string, phase?: string): MoveProgressLogEntry {
  return {
    id: createClientId(),
    message,
    phase,
  };
}

function currentLanguage(): string {
  const stored = typeof window !== 'undefined' ? localStorage.getItem('mcharbor-language') : null;
  if (!stored) return 'en';
  try {
    return JSON.parse(stored)?.state?.language || 'en';
  } catch {
    return stored;
  }
}

export function useMoveContainerStream() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');
  const [moving, setMoving] = useState(false);
  const [progress, setProgress] = useState<MoveContainerEvent | null>(null);
  const [logs, setLogs] = useState<MoveProgressLogEntry[]>([]);
  const abortRef = useRef<AbortController | null>(null);

  const invalidateMoveQueries = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['containers'] });
    queryClient.invalidateQueries({ queryKey: ['container'] });
    queryClient.invalidateQueries({ queryKey: ['images'] });
    queryClient.invalidateQueries({ queryKey: ['volumes'] });
    queryClient.invalidateQueries({ queryKey: ['networks'] });
  }, [queryClient]);

  const reset = useCallback(() => {
    setMoving(false);
    setProgress(null);
    setLogs([]);
  }, []);

  const abort = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const startMove = useCallback(
    (options: MoveContainerOptions, callbacks?: { onDone?: () => void }) => {
      const { id, ...body } = options;
      const envQuery = envId ? `?env=${envId}` : '';
      const controller = new AbortController();
      abortRef.current = controller;

      setMoving(true);
      setProgress({ step: 0, total: 10, message: t('moveDialog.progress.starting'), status: 'progress', phase: 'start' });
      setLogs([createMoveLogEntry(t('moveDialog.progress.starting'), 'start')]);

      fetch(`/api/containers/${id}/move/stream${envQuery}`, {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          'Accept-Language': currentLanguage(),
        },
        body: JSON.stringify(body),
        signal: controller.signal,
      })
        .then((res) => {
          if (!res.ok || !res.body) {
            setMoving(false);
            setProgress({ step: 0, total: 10, message: t('moveDialog.progress.connectFailed'), status: 'error', phase: 'error' });
            setLogs((prev) => [...prev, createMoveLogEntry(t('moveDialog.progress.connectFailed'), 'error')]);
            return;
          }

          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buffer = '';
          let completed = false;

          const read = (): Promise<void> =>
            reader.read().then(({ done, value }) => {
              if (done) {
                setMoving(false);
                if (!completed) {
                  setProgress({ step: 0, total: 10, message: t('moveDialog.progress.networkError'), status: 'error', phase: 'error' });
                  setLogs((prev) => [...prev, createMoveLogEntry(t('moveDialog.progress.networkError'), 'error')]);
                }
                return;
              }

              buffer += decoder.decode(value, { stream: true });
              const lines = buffer.split('\n');
              buffer = lines.pop() ?? '';

              for (const line of lines) {
                if (!line.startsWith('data: ')) continue;
                try {
                  const event = JSON.parse(line.slice(6)) as MoveContainerEvent;
                  setProgress(event);
                  if (event.message) {
                    setLogs((prev) => [...prev, createMoveLogEntry(event.message, event.phase)]);
                  }
                  if (event.status === 'done') {
                    completed = true;
                    setMoving(false);
                    invalidateMoveQueries();
                    callbacks?.onDone?.();
                    return;
                  }
                  if (event.status === 'error') {
                    completed = true;
                    setMoving(false);
                    return;
                  }
                } catch {
                  // Skip malformed SSE lines.
                }
              }

              return read();
            });

          return read();
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === 'AbortError') return;
          setMoving(false);
          setProgress({ step: 0, total: 10, message: t('moveDialog.progress.networkError'), status: 'error', phase: 'error' });
          setLogs((prev) => [...prev, createMoveLogEntry(t('moveDialog.progress.networkError'), 'error')]);
        });
    },
    [envId, invalidateMoveQueries, t],
  );

  return { moving, progress, logs, startMove, abort, reset };
}

type RemoveContainerOptions = {
  id: string;
  force: boolean;
  removeVolumes: boolean;
  removeImage: boolean;
  removeStack: boolean;
};

export function useRemoveContainer() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, ...body }: RemoveContainerOptions) => {
      const envQuery = envId ? `?env=${envId}` : '';
      return api.post(`/containers/${id}/remove${envQuery}`, body).then(assertSuccess);
    },
    meta: { success: t('toast.removed') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['containers'] });
      queryClient.invalidateQueries({ queryKey: ['container'] });
      queryClient.invalidateQueries({ queryKey: ['stacks'] });
      queryClient.invalidateQueries({ queryKey: ['images'] });
    },
  });
}

export function usePruneContainers() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: () =>
      api.post(`/containers/prune${envId ? `?env=${envId}` : ''}`, {}).then(assertSuccess),
    meta: { success: t('toast.pruned') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['containers'] });
    },
  });
}
