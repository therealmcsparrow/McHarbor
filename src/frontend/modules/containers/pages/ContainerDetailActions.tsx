// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconBlocks,
  IconPlayerPause,
  IconSkull,
  IconTrash,
  IconExternalLink,
  IconArrowsTransferUp,
  IconArrowsExchange,
  IconLink,
  IconLock,
  IconLockOpen,
  IconTag,
  IconSettings,
} from '@tabler/icons-react';
import { PageHeadActions } from '@resources/layout/PageHeadIdentity';
import type { ContainerInspect } from '@core/types/docker';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { PERM } from '@core/auth/permissionKeys';
import { useHostSelfGate } from '../hooks/useHostSelfGate';

interface ContainerDetailActionsProps {
  container: ContainerInspect;
  isRunning: boolean;
  isPaused: boolean;
  webUrl: string | null;
  protectedContainer: boolean;
  editUnlocked: boolean;
  editUnlockable: boolean;
  actionsLocked: boolean;
  onToggleEditLock: () => void;
  onOpenConfigEditor: () => void;
  onRename: () => void;
  onAction: (action: string) => void;
  onRecreate: () => void;
  onKill: () => void;
  onRemove: () => void;
  onMove: () => void;
  onTakeOver: () => void;
  onRelink: () => void;
}

export function ContainerDetailActions(props: ContainerDetailActionsProps) {
  const {
    container,
    isRunning,
    isPaused,
    webUrl,
    protectedContainer,
    editUnlocked,
    editUnlockable,
    actionsLocked,
    onToggleEditLock,
    onOpenConfigEditor,
    onRename,
    onAction,
    onRecreate,
    onKill,
    onRemove,
    onMove,
    onTakeOver,
    onRelink,
  } = props;
  const { t } = useTranslation('containers');
  const gate = useHostSelfGate(container);
  const state = container.State?.Status ?? 'unknown';
  // Unlocking a protected container frees its actions as well as its
  // editing — the lock is one decision, not two.
  const actionLocked = (protectedContainer && !editUnlocked) || actionsLocked;
  const canStart = state === 'created' || state === 'exited';
  const canRestart = isRunning || isPaused || state === 'exited';
  const canMove = isRunning || isPaused || canStart;
  return (
    <PageHeadActions>
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        {/* The toggle stays mounted in both states — it is the only way
            back to locked. The icon reports the current state (open and
            green when unlocked, closed and red when locked); the tooltip
            names the action the click performs. */}
        {protectedContainer && editUnlockable && (
          <HeaderActionButton
            tooltip={editUnlocked ? t('actions.lockEditing') : t('actions.unlockEditing')}
            onClick={onToggleEditLock}
            variant={editUnlocked ? 'secondary' : 'outline'}
            icon={
              editUnlocked ? (
                <IconLockOpen className="size-3.5 text-success" />
              ) : (
                <IconLock className="size-3.5 text-danger" />
              )
            }
          />
        )}
        <HeaderActionButton
          tooltip={t('actions.rename')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={onRename}
          disabled={actionLocked}
          icon={<IconTag className="size-3.5" />}
        />
        <HeaderActionButton
          tooltip={t('actions.openConfigEditor')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={onOpenConfigEditor}
          disabled={actionsLocked}
          icon={<IconSettings className="size-3.5" />}
        />
        {isRunning ? (
          <HeaderActionButton
            tooltip={t('actions.stop')}
            permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
            onClick={() => onAction('stop')}
            disabled={actionLocked}
            icon={<IconPlayerStop className="size-3.5" />}
          />
        ) : isPaused ? (
          <HeaderActionButton
            tooltip={t('actions.resume')}
            permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
            onClick={() => onAction('unpause')}
            variant="default"
            disabled={actionLocked}
            icon={<IconPlayerPlay className="size-3.5" />}
          />
        ) : canStart ? (
          <HeaderActionButton
            tooltip={t('actions.start')}
            permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
            onClick={() => onAction('start')}
            variant="default"
            disabled={actionLocked}
            icon={<IconPlayerPlay className="size-3.5" />}
          />
        ) : null}
        {canRestart && (
          <HeaderActionButton
            tooltip={t('actions.restart')}
            permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
            onClick={() => onAction('restart')}
            disabled={actionLocked}
            icon={<IconRotate className="size-3.5" />}
          />
        )}
        {/* Recreate sits next to restart because that is what users reach
            for when a restart is not enough — it replaces the container from
            its current image and configuration rather than just bouncing it. */}
        <HeaderActionButton
          tooltip={t('actions.recreate')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={onRecreate}
          disabled={actionLocked}
          icon={<IconBlocks className="size-3.5" />}
        />
        <HeaderActionButton
          tooltip={t('actions.pause')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={() => onAction('pause')}
          disabled={actionLocked || !isRunning}
          icon={<IconPlayerPause className="size-3.5" />}
        />
        {webUrl && (
          <HeaderActionButton
            tooltip={t('actions.openWebsite')}
            onClick={() => window.open(webUrl, '_blank', 'noopener,noreferrer')}
            variant="secondary"
            icon={<IconExternalLink className="size-3.5" />}
          />
        )}
        <HeaderActionButton
          tooltip={t('actions.relinkStack')}
          permission={PERM.CONTAINERS_MANAGE}
          onClick={onRelink}
          variant="secondary"
          disabled={actionLocked}
          icon={<IconLink className="size-3.5" />}
        />
        <HeaderActionButton
          tooltip={t('actions.move')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={onMove}
          variant="secondary"
          disabled={actionLocked || !canMove}
          icon={<IconArrowsExchange className="size-3.5" />}
        />
        {!container.Config?.Labels?.['com.docker.compose.project'] && (
          <HeaderActionButton
            tooltip={t('actions.takeOver')}
            onClick={onTakeOver}
            variant="secondary"
            disabled={actionLocked || !canMove}
            icon={<IconArrowsTransferUp className="size-3.5" />}
          />
        )}
        <div className="mx-0.5 h-5 w-px bg-border" />
        <HeaderActionButton
          tooltip={t('actions.kill')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={onKill}
          variant="destructive"
          disabled={actionLocked || (!isRunning && !isPaused)}
          icon={<IconSkull className="size-3.5" />}
        />
        <HeaderActionButton
          tooltip={t('actions.remove')}
          permission={gate.lifecycle(PERM.CONTAINERS_DELETE)}
          onClick={onRemove}
          variant="destructive"
          disabled={actionLocked}
          icon={<IconTrash className="size-3.5" />}
        />
      </div>
    </PageHeadActions>
  );
}
