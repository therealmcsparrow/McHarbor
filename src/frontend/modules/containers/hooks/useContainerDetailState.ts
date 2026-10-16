// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useCallback, useState } from 'react';
import { useIsMutating } from '@tanstack/react-query';
import type { ContainerInspect } from '@core/types/docker';
import { isMcHarborContainer, isProtectedContainer } from '@core/utils/protection';
import { CONTAINER_ACTION_MUTATION_KEY, useContainerAction } from './useContainers';

/** Derives what the container's state and protection allow the operator to do. */
export function useContainerDetailPermissions(container: ContainerInspect | undefined, editUnlocked: boolean) {
  const actionPending = useIsMutating({ mutationKey: CONTAINER_ACTION_MUTATION_KEY }) > 0;
  const isMcHarborTarget = container ? isMcHarborContainer(container) : false;
  const containerName = (container?.Name ?? '').replace(/^\//, '');
  // The flag lives on the container, not in any request we still hold. It is
  // only worth offering where it would change something: a container with no
  // secret references has nothing to move into a file.
  const containerSecretFileMode = container?.Config?.Labels?.['com.mcharbor.secret-files'] === 'true';
  const secretFileModeOffered =
    containerSecretFileMode || Boolean(container?.Config?.Labels?.['com.mcharbor.secret-refs']);
  const containerState = container?.State?.Status ?? 'unknown';
  const isRunning = containerState === 'running';
  const isPaused = containerState === 'paused';
  const canStart = containerState === 'created' || containerState === 'exited';
  const canRestart = isRunning || isPaused || containerState === 'exited';
  const stateAllowsActions = isRunning || isPaused || canStart;
  const protectedContainer = container ? isProtectedContainer(container) : true;
  // Unlocking a McHarbor-managed container frees its actions too, not just
  // its editing — the lock is one decision.
  const editUnlockActive = isMcHarborTarget && editUnlocked;
  const actionsLocked = (protectedContainer && !editUnlockActive) || actionPending;
  return {
    actionPending,
    isMcHarborTarget,
    containerName,
    containerSecretFileMode,
    secretFileModeOffered,
    isRunning,
    isPaused,
    canStart,
    canRestart,
    stateAllowsActions,
    protectedContainer,
    editUnlockActive,
    actionsLocked,
  };
}

type DetailPermissions = ReturnType<typeof useContainerDetailPermissions>;

/** Owns the detail page's dialog open-state and the guarded container action handler. */
export function useContainerDetailDialogs(container: ContainerInspect | undefined, permissions: DetailPermissions) {
  const { actionsLocked, canStart, canRestart, isRunning, isPaused } = permissions;
  const { mutate: runContainerAction } = useContainerAction();
  const containerId = container?.Id;
  const [confirmStop, setConfirmStop] = useState(false);
  const [confirmKill, setConfirmKill] = useState(false);
  const [recreateConfirmOpen, setRecreateConfirmOpen] = useState(false);
  const [secretFileMode, setSecretFileMode] = useState(false);
  const [removeDialogOpen, setRemoveDialogOpen] = useState(false);
  const [moveDialogOpen, setMoveDialogOpen] = useState(false);
  const [renameDialogOpen, setRenameDialogOpen] = useState(false);
  const [takeOverOpen, setTakeOverOpen] = useState(false);
  const [relinkOpen, setRelinkOpen] = useState(false);

  const handleContainerAction = useCallback(
    (nextAction: string) => {
      if (actionsLocked || !containerId) return;
      if (
        (nextAction === 'start' && !canStart) ||
        (nextAction === 'restart' && !canRestart) ||
        (nextAction === 'pause' && !isRunning) ||
        (nextAction === 'unpause' && !isPaused) ||
        (nextAction === 'stop' && !isRunning && !isPaused)
      ) {
        return;
      }
      if (nextAction === 'stop') {
        setConfirmStop(true);
        return;
      }
      runContainerAction({ id: containerId, action: nextAction });
    },
    [actionsLocked, canRestart, canStart, containerId, isPaused, isRunning, runContainerAction],
  );
  const openStopDialog = useCallback(() => setConfirmStop(true), []);
  const openKillDialog = useCallback(() => setConfirmKill(true), []);
  const openMoveDialog = useCallback(() => setMoveDialogOpen(true), []);
  const openRenameDialog = useCallback(() => setRenameDialogOpen(true), []);
  const closeAllDialogs = useCallback(() => {
    setConfirmStop(false);
    setConfirmKill(false);
    setRecreateConfirmOpen(false);
    setRemoveDialogOpen(false);
    setMoveDialogOpen(false);
    setRenameDialogOpen(false);
    setTakeOverOpen(false);
    setRelinkOpen(false);
  }, []);

  return {
    confirmStop,
    confirmKill,
    recreateConfirmOpen,
    secretFileMode,
    removeDialogOpen,
    moveDialogOpen,
    renameDialogOpen,
    takeOverOpen,
    relinkOpen,
    setConfirmStop,
    setConfirmKill,
    setRecreateConfirmOpen,
    setSecretFileMode,
    setRemoveDialogOpen,
    setMoveDialogOpen,
    setRenameDialogOpen,
    setTakeOverOpen,
    setRelinkOpen,
    handleContainerAction,
    openStopDialog,
    openKillDialog,
    openMoveDialog,
    openRenameDialog,
    closeAllDialogs,
  };
}
