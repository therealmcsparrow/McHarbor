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

export type ServiceUpdateResult = {
  serviceName: string;
  containerId?: string;
  image: string;
  currentDigest: string;
  remoteDigest: string;
  updateAvailable: boolean;
  error?: string;
};

export type StackUpdateResult = {
  stackName: string;
  updateAvailable: boolean;
  services: ServiceUpdateResult[];
};

export type StackOperationMode = 'update' | 'reinstall';

export type StackOperationTarget = {
  name: string;
  images: string[];
};

type ComposeResult = {
  success: boolean;
  output?: string;
  error?: string;
};

export type StackOperationResult = {
  detail: string;
  output?: string;
  images: string[];
};

type OperationScansApi = ReturnType<typeof useOperationScans>;
type OperationScanner = ReturnType<OperationScansApi['createBatchScanner']>;

type StackOperationOptions = {
  log?: BatchProgressContext['log'];
  scanner?: OperationScanner;
};

const selfUpdateRecoveryTimeoutMs = 60_000;
const selfUpdatePollIntervalMs = 2_000;

function isSelfUpdateTarget(target: StackOperationTarget) {
  return target.name.toLowerCase() === 'mcharbor'
    || target.images.some((image) => image.toLowerCase().includes('/mcharbor'));
}

async function delay(ms: number) {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForSelfUpdateRecovery(
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
        return;
      }
    } catch {
      // The app is expected to be unavailable while the container restarts.
    }

    await delay(selfUpdatePollIntervalMs);
  }

  throw new Error(t('operations.log.apiRecoveryTimeout'));
}

export function useCheckStackUpdates() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('stacks');

  return useMutation({
    mutationFn: (stackNames?: string[]) =>
      api
        .post<StackUpdateResult[]>(
          `/stacks/check-updates${envId ? `?env=${envId}` : ''}`,
          stackNames ? { stackNames } : {}
        )
        .then((r) => r.data ?? []),
    meta: { success: t('updates.checkComplete') },
    onSuccess: (data) => {
      // Store update results in the query cache for columns to read
      queryClient.setQueryData(['stack-updates', envId], () => {
        const map = new Map<string, StackUpdateResult>();
        for (const r of data) map.set(r.stackName, r);
        return map;
      });
    },
  });
}

export function useStackUpdateResults() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useReactiveQueryData<Map<string, StackUpdateResult>>(['stack-updates', envId]);
}

export function useStackOperationActions() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const operationScans = useOperationScans();

  async function runOperation(
    target: StackOperationTarget,
    mode: StackOperationMode,
    options: StackOperationOptions = {},
  ): Promise<StackOperationResult> {
    const { log, scanner = operationScans.createBatchScanner() } = options;
    const envQuery = envId ? `?env=${envId}` : '';

    log?.(tc('operations.log.preparing'), {
      detail: tc('operations.log.preparing'),
    });
    log?.(
      mode === 'update'
        ? tc('operations.log.redeployingStack')
        : tc('operations.log.forceRecreatingStack'),
      {
        detail:
          mode === 'update'
            ? tc('operations.log.redeployingStack')
            : tc('operations.log.forceRecreatingStack'),
      },
    );

    let result: ComposeResult;
    try {
      result = await api
        .post<ComposeResult>(`/stacks/${target.name}/${mode}${envQuery}`)
        .then(assertSuccess);
    } catch (error) {
      if (!isSelfUpdateTarget(target)) {
        throw error;
      }

      await waitForSelfUpdateRecovery(tc, log);
      result = { success: true };
    }

    const outputLines = extractLogTail(result.output ?? '', 30);
    if (outputLines.length > 0) {
      log?.(tc('operations.log.composeOutput'));
      for (const line of outputLines) {
        log?.(line);
      }
    }

    const scanLog = (message: string, scanOptions?: { level?: BatchProgressLogLevel }) => {
      log?.(message, scanOptions);
    };
    await scanner.runScansForImages(target.images, scanLog);

    return {
      detail: mode === 'update' ? t('toast.updated') : t('toast.reinstalled'),
      output: result.output,
      images: target.images,
    };
  }

  async function finalizeOperation(mode: StackOperationMode, targets: StackOperationTarget[]) {
    if (targets.length === 0) {
      return;
    }

    if (mode === 'update') {
      const updatedNames = new Set(targets.map((target) => target.name));
      queryClient.setQueryData<Map<string, StackUpdateResult> | undefined>(
        ['stack-updates', envId],
        (current) => {
          if (!current) return current;
          const next = new Map(current);
          for (const name of updatedNames) {
            next.delete(name);
          }
          return next;
        },
      );
    }

    queryClient.removeQueries({ queryKey: ['container-updates', envId] });

    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['stacks'] }),
      queryClient.invalidateQueries({ queryKey: ['stack'] }),
      queryClient.invalidateQueries({ queryKey: ['stack-containers'] }),
      queryClient.invalidateQueries({ queryKey: ['containers'] }),
      queryClient.invalidateQueries({ queryKey: ['images'] }),
    ]);
  }

  return {
    runOperation,
    finalizeOperation,
    createBatchScanner: operationScans.createBatchScanner,
  };
}
