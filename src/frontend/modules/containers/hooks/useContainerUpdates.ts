// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type {
  BatchProgressContext,
  BatchProgressLogLevel,
} from '@resources/hooks/useBatchProgressOperation';
import { useOperationScans } from '@resources/hooks/useOperationScans';
import { useReactiveQueryData } from '@resources/hooks/useReactiveQueryData';
import { assertSuccess } from '@resources/utils/api-mutation';
import { extractLogTail } from '@resources/utils/log-tail';
import { useEnvironmentStore } from '@resources/stores/environment';

export type ImageUpdateResult = {
  containerId: string;
  containerName: string;
  image: string;
  currentDigest: string;
  remoteDigest: string;
  updateAvailable: boolean;
  error?: string;
};

export type ContainerOperationMode = 'update' | 'reinstall';

export type ContainerOperationTarget = {
  id: string;
  name: string;
  image: string;
};

type RecreateResponse = {
  Id: string;
};

export type ContainerOperationResult = {
  detail: string;
  newContainerId: string;
  image: string;
};

type OperationScansApi = ReturnType<typeof useOperationScans>;
type OperationScanner = ReturnType<OperationScansApi['createBatchScanner']>;

type ContainerOperationOptions = {
  log?: BatchProgressContext['log'];
  scanner?: OperationScanner;
};

const selfUpdateRecoveryTimeoutMs = 60_000;
const selfUpdatePollIntervalMs = 2_000;

function isSelfUpdateTarget(target: ContainerOperationTarget) {
  const normalizedName = target.name.replace(/^\//, '').toLowerCase();
  const normalizedImage = target.image.toLowerCase();

  return normalizedName === 'mcharbor' || normalizedImage.includes('/mcharbor');
}

async function delay(ms: number) {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForSelfUpdateRecovery(
  target: ContainerOperationTarget,
  envQuery: string,
  t: (key: string, options?: Record<string, unknown>) => string,
  log?: BatchProgressContext['log'],
) {
  const deadline = Date.now() + selfUpdateRecoveryTimeoutMs;
  const waitingMessage = t('operations.log.waitingForApiRecovery');
  log?.(waitingMessage, {
    detail: waitingMessage,
  });

  while (Date.now() < deadline) {
    try {
      const health = await fetch('/api/health', { credentials: 'include' });
      if (health.ok) {
        const containers = await api
          .get<
            Array<{
              Id?: string;
              ID?: string;
              Names?: string[];
              Image?: string;
            }>
          >(`/containers${envQuery}${envQuery ? '&' : '?'}all=true`)
          .then(assertSuccess);

        const match = containers.find((container) => {
          const names = container.Names ?? [];
          const byName = names.some(
            (name) => name.replace(/^\//, '').toLowerCase() === target.name.replace(/^\//, '').toLowerCase(),
          );
          const byImage = (container.Image ?? '').toLowerCase() === target.image.toLowerCase();
          return byName || byImage;
        });

        return match?.Id ?? match?.ID ?? target.id;
      }
    } catch {
      // The app is expected to be unavailable while the container restarts.
    }

    await delay(selfUpdatePollIntervalMs);
  }

  throw new Error(t('operations.log.apiRecoveryTimeout'));
}

export function useCheckContainerUpdates() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (containerIds?: string[]) =>
      api
        .post<ImageUpdateResult[]>(
          `/containers/check-updates${envId ? `?env=${envId}` : ''}`,
          containerIds ? { containerIds } : {}
        )
        .then((r) => r.data ?? []),
    meta: { success: t('updates.checkComplete') },
    onSuccess: (data) => {
      // Store update results in the query cache for the update column to read
      queryClient.setQueryData(['container-updates', envId], () => {
        const map = new Map<string, ImageUpdateResult>();
        for (const r of data) map.set(r.containerId, r);
        return map;
      });
    },
  });
}

export function useContainerOperationActions() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const operationScans = useOperationScans();

  async function runOperation(
    target: ContainerOperationTarget,
    mode: ContainerOperationMode,
    options: ContainerOperationOptions = {},
  ): Promise<ContainerOperationResult> {
    const { log, scanner = operationScans.createBatchScanner() } = options;
    const envQuery = envId ? `?env=${envId}` : '';

    log?.(tc('operations.log.preparing'), {
      detail: tc('operations.log.preparing'),
    });
    log?.(
      mode === 'update'
        ? tc('operations.log.pullingLatestImage')
        : tc('operations.log.usingCurrentImage'),
      {
        detail:
          mode === 'update'
            ? tc('operations.log.pullingLatestImage')
            : tc('operations.log.usingCurrentImage'),
      },
    );
    log?.(tc('operations.log.recreatingContainer'), {
      detail: tc('operations.log.recreatingContainer'),
    });

    let recreated: RecreateResponse;
    try {
      recreated = await api
        .post<RecreateResponse>(`/containers/${target.id}/recreate${envQuery}`, {
          pullImage: mode === 'update',
        })
        .then(assertSuccess);
    } catch (error) {
      if (!isSelfUpdateTarget(target)) {
        throw error;
      }

      const recoveredContainerId = await waitForSelfUpdateRecovery(target, envQuery, tc, log);
      recreated = { Id: recoveredContainerId };
    }

    log?.(tc('operations.log.fetchingRecentLogs'), {
      detail: tc('operations.log.fetchingRecentLogs'),
    });

    try {
      const logs = await api
        .get<{ logs: string }>(`/containers/${recreated.Id}/logs`, {
          stdout: 'true',
          stderr: 'true',
          tail: '20',
        })
        .then(assertSuccess);
      const lines = extractLogTail(logs.logs, 20);
      if (lines.length > 0) {
        log?.(tc('operations.log.startupLogs'));
        for (const line of lines) {
          log?.(line);
        }
      } else {
        log?.(tc('operations.log.startupLogsUnavailable'), { level: 'warning' });
      }
    } catch {
      log?.(tc('operations.log.startupLogsUnavailable'), { level: 'warning' });
    }

    const scanLog = (message: string, scanOptions?: { level?: BatchProgressLogLevel }) => {
      log?.(message, scanOptions);
    };
    await scanner.runScansForImages([target.image], scanLog);

    return {
      detail: mode === 'update' ? t('toast.updated') : t('toast.reinstalled'),
      newContainerId: recreated.Id,
      image: target.image,
    };
  }

  async function finalizeOperation(
    mode: ContainerOperationMode,
    targets: ContainerOperationTarget[],
  ) {
    if (targets.length === 0) {
      return;
    }

    if (mode === 'update') {
      const updatedIds = new Set(targets.map((target) => target.id));
      queryClient.setQueryData<Map<string, ImageUpdateResult> | undefined>(
        ['container-updates', envId],
        (current) => {
          if (!current) return current;
          const next = new Map(current);
          for (const id of updatedIds) {
            next.delete(id);
          }
          return next;
        },
      );
    }

    queryClient.removeQueries({ queryKey: ['stack-updates', envId] });

    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['containers'] }),
      queryClient.invalidateQueries({ queryKey: ['container'] }),
      queryClient.invalidateQueries({ queryKey: ['images'] }),
      queryClient.invalidateQueries({ queryKey: ['stacks'] }),
      queryClient.invalidateQueries({ queryKey: ['stack'] }),
    ]);
  }

  return {
    runOperation,
    finalizeOperation,
    createBatchScanner: operationScans.createBatchScanner,
  };
}

export function useContainerUpdateResults() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useReactiveQueryData<Map<string, ImageUpdateResult>>(['container-updates', envId]);
}
