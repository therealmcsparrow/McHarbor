// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import {
  useCancelContainerBackupRun,
  useContainerBackupRestoreOptions,
  useContainerBackupRun,
  useRestoreContainerBackup,
  type BackupSelectionMode,
  type ContainerBackupRun,
} from '@resources/hooks/useContainerBackups';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';

export function useRestoreRunState(
  run: ContainerBackupRun | null,
  containerId: string,
  onClose: () => void,
  initialSecretKey = '',
) {
  // The option lists come from four restore.view-gated routes; without the key
  // every one 403s, so the dialog names the missing permission instead of
  // firing four failing requests behind a spinner.
  const canViewRestoreOptions = usePermission(PERM.RESTORE_VIEW);
  const restoreBackup = useRestoreContainerBackup(containerId);
  const cancelRun = useCancelContainerBackupRun();
  const [restoreSecretKey, setRestoreSecretKey] = useState(run?.id ? initialSecretKey : '');
  const [restoreSelectedItems, setRestoreSelectedItems] = useState<string[]>([]);
  // Each layer has its own off / choose / full mode, defaulting to full. When
  // every layer is full an empty restoreItems list is sent and the backend
  // restores everything; otherwise the precise selection is sent, with a
  // whole-unit key standing in for each full layer.
  const [restoreModes, setRestoreModes] = useState<Record<string, BackupSelectionMode>>({});
  const modeFor = (key: string): BackupSelectionMode => restoreModes[key] ?? 'full';
  const setModeFor = (key: string, mode: BackupSelectionMode) =>
    setRestoreModes((current) => ({ ...current, [key]: mode }));
  const [restoreRunId, setRestoreRunId] = useState<string | null>(null);

  const runId = run?.id;
  const resetKey = `${runId ?? ''}|${initialSecretKey}`;
  const [appliedResetKey, setAppliedResetKey] = useState(resetKey);
  if (appliedResetKey !== resetKey) {
    setAppliedResetKey(resetKey);
    if (runId) {
      setRestoreSecretKey(initialSecretKey);
      setRestoreSelectedItems([]);
      setRestoreModes({});
      setRestoreRunId(null);
    }
  }

  const restoreOptionsEnabled =
    canViewRestoreOptions && !!run && (!run.requiresSecretKey || restoreSecretKey.trim() !== '');
  const {
    data: restoreOptions,
    isFetching: restoreOptionsLoading,
    isError: restoreOptionsError,
  } = useContainerBackupRestoreOptions(run?.id, restoreSecretKey, restoreOptionsEnabled);
  const { data: restoreRun, isLoading: restoreRunLoading } = useContainerBackupRun(
    containerId,
    restoreRunId ?? undefined,
  );
  const restoreIsActive = restoreRun?.status === 'running';

  const filesystemItem = restoreOptions?.items.find((item) => item.type === 'filesystem');
  const configItem = restoreOptions?.items.find((item) => item.type === 'config');
  const mountItems = restoreOptions?.items.filter((item) => item.type === 'mount') ?? [];
  const imageAndLogItems = restoreOptions?.items.filter((item) => item.type === 'image' || item.type === 'logs') ?? [];

  const layerKeys = [
    ...(filesystemItem ? ['filesystem'] : []),
    ...(configItem ? ['config'] : []),
    ...mountItems.map((item) => item.key),
  ];
  const allFull = layerKeys.every((key) => modeFor(key) === 'full');
  // Layers left on full still restore completely in a mixed selection; layers
  // set to off contribute nothing, and a choose layer's tree writes its own
  // keys into restoreSelectedItems.
  const fullLayerKeys = layerKeys.filter((key) => modeFor(key) === 'full');

  function closeRestoreDialog() {
    onClose();
    setRestoreRunId(null);
    setRestoreSecretKey('');
    setRestoreSelectedItems([]);
  }

  function toggleRestoreItem(key: string, checked: boolean) {
    setRestoreSelectedItems((current) => {
      if (checked) {
        return current.includes(key) ? current : [...current, key];
      }
      return current.filter((item) => item !== key);
    });
  }

  function startRestore() {
    if (!run) return;
    const itemsToSend = allFull ? [] : Array.from(new Set([...fullLayerKeys, ...restoreSelectedItems]));
    restoreBackup.mutate(
      {
        id: run.id,
        secretKey: restoreSecretKey,
        restoreItems: itemsToSend,
      },
      {
        onSuccess: (newRun) => {
          setRestoreRunId(newRun.id);
        },
      },
    );
  }

  return {
    canViewRestoreOptions,
    restoreBackup,
    cancelRun,
    restoreSecretKey,
    setRestoreSecretKey,
    restoreSelectedItems,
    setRestoreSelectedItems,
    modeFor,
    setModeFor,
    restoreRunId,
    restoreOptions,
    restoreOptionsEnabled,
    restoreOptionsLoading,
    restoreOptionsError,
    restoreRun,
    restoreRunLoading,
    restoreIsActive,
    filesystemItem,
    configItem,
    mountItems,
    imageAndLogItems,
    allFull,
    closeRestoreDialog,
    toggleRestoreItem,
    startRestore,
  };
}

export type RestoreRunState = ReturnType<typeof useRestoreRunState>;
