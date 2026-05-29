// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { toast } from "sonner";
import type { TFunction } from "i18next";
import { useBatchProgressOperation } from "@resources/hooks/useBatchProgressOperation";
import { isProtectedStack } from "@core/utils/protection";
import type { StackInfo } from "./useStacks";
import {
  useStackOperationActions,
  useStackUpdateResults,
  type StackOperationMode,
  type StackOperationTarget,
} from "./useStackUpdates";

function toStackTarget(stack: StackInfo): StackOperationTarget {
  return {
    name: stack.name,
    images: [
      ...new Set(
        stack.services.map((service) => service.image).filter(Boolean),
      ),
    ],
  };
}

export function useStackBatchOperations(
  stacks: StackInfo[],
  t: TFunction<"stacks">,
) {
  const stackOperations = useStackOperationActions();
  const batchProgress = useBatchProgressOperation();
  const updateResults = useStackUpdateResults();

  const managedStacks = stacks.filter((stack) => stack.type === "managed" && !isProtectedStack(stack));
  const updateAvailableTargets = managedStacks
    .filter((stack) => updateResults?.get(stack.name)?.updateAvailable)
    .map(toStackTarget);
  const reinstallTargets = managedStacks.map(toStackTarget);

  function getManagedTargets(rows: StackInfo[]) {
    const managedRows = rows.filter((row) => row.type === "managed" && !isProtectedStack(row));
    if (managedRows.length !== rows.length) {
      toast.warning(t("updates.managedOnly"));
    }
    return managedRows.map(toStackTarget);
  }

  async function runStackOperation(
    mode: StackOperationMode,
    targets: StackOperationTarget[],
  ) {
    if (targets.length === 0) {
      return;
    }

    const batchScanner = stackOperations.createBatchScanner();
    await batchProgress.runBatchOperation({
      title: t(`updates.progress.${mode}Title`),
      description: t(`updates.progress.${mode}Description`),
      actionLabel: t(`updates.progress.${mode}Action`),
      items: targets,
      getKey: (target) => target.name,
      getLabel: (target) => target.name,
      execute: async (target, progress) => {
        const result = await stackOperations.runOperation(target, mode, {
          log: progress.log,
          scanner: batchScanner,
        });
        return { detail: result.detail };
      },
      onComplete: async (result) => {
        await stackOperations.finalizeOperation(mode, result.successfulItems);
      },
      getSuccessToast: (result) =>
        result.successCount === 1
          ? mode === "update"
            ? t("toast.updated")
            : t("toast.reinstalled")
          : mode === "update"
            ? t("updates.bulkUpdated", { count: result.successCount })
            : t("updates.bulkReinstalled", { count: result.successCount }),
      getErrorToast: (result) =>
        t("updates.partialComplete", {
          success: result.successCount,
          total: result.total,
        }),
    });
  }

  return {
    batchProgress,
    updateResults,
    updateAvailableTargets,
    reinstallTargets,
    getManagedTargets,
    runStackOperation,
  };
}
