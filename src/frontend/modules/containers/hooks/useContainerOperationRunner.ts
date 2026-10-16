// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Running an update or recreate across a set of containers: building the
// targets, driving the batch-progress dialog, and reporting the result. Lifted
// out of ContainersPage, which had it inline alongside its rendering.

import { useTranslation } from 'react-i18next';
import type { ContainerInfo } from '@core/types/docker';
import { canRunContainerUpdateOperation } from '@core/utils/protection';
import { useBatchProgressOperation } from '@resources/hooks/useBatchProgressOperation';
import {
  useContainerOperationActions,
  type ContainerOperationMode,
  type ContainerOperationTarget,
} from './useContainerUpdates';

export function toContainerTarget(container: ContainerInfo): ContainerOperationTarget {
  return {
    id: container.Id,
    name: container.Names?.[0]?.replace(/^\//, '') ?? container.Id,
    image: container.Image,
  };
}

export function useContainerOperationRunner(
  containers: ContainerInfo[],
  updateAvailableIDs: Set<string>,
  batchProgress: ReturnType<typeof useBatchProgressOperation>,
) {
  const { t } = useTranslation('containers');
  const containerOperations = useContainerOperationActions();

  const updateOperationContainers = containers.filter(canRunContainerUpdateOperation);
  const allTargets = updateOperationContainers.map(toContainerTarget);
  const updateTargets = updateOperationContainers
    .filter((container) => updateAvailableIDs.has(container.Id))
    .map(toContainerTarget);

  async function runContainerOperation(mode: ContainerOperationMode, targets: ContainerOperationTarget[]) {
    if (targets.length === 0) {
      return;
    }

    const batchScanner = containerOperations.createBatchScanner();
    await batchProgress.runBatchOperation({
      title: t(`updates.progress.${mode}Title`),
      description: t(`updates.progress.${mode}Description`),
      actionLabel: t(`updates.progress.${mode}Action`),
      items: targets,
      getKey: (target) => target.id,
      getLabel: (target) => target.name,
      execute: async (target, progress) => {
        const result = await containerOperations.runOperation(target, mode, {
          log: progress.log,
          scanner: batchScanner,
        });
        return { detail: result.detail };
      },
      onComplete: async (result) => {
        await containerOperations.finalizeOperation(mode, result.successfulItems);
      },
      getSuccessToast: (result) =>
        result.successCount === 1
          ? mode === 'update'
            ? t('toast.updated')
            : t('toast.recreated')
          : mode === 'update'
            ? t('updates.bulkUpdated', { count: result.successCount })
            : t('updates.bulkRecreated', { count: result.successCount }),
      getErrorToast: (result) =>
        t('updates.partialComplete', {
          success: result.successCount,
          total: result.total,
        }),
    });
  }

  return { allTargets, updateTargets, runContainerOperation };
}
