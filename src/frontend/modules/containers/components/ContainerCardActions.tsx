// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconBlocks,
  IconTrash,
  IconExternalLink,
  IconTerminal2,
  IconFileText,
  IconTag,
  IconArrowsExchange,
} from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import { CardFooter } from '@resources/components/ui/Card';
import { ActionButton } from '@resources/components/ui/ActionButton';
import { PERM } from '@core/auth/permissionKeys';
import { useHostSelfGate } from '../hooks/useHostSelfGate';

type ContainerCardActionsProps = {
  container: ContainerInfo;
  locked: boolean;
  webUrl: string | null;
  onAction: (id: string, action: string) => void;
  onTerminal: (c: ContainerInfo) => void;
  onLogs: (c: ContainerInfo) => void;
  onRename: (c: ContainerInfo) => void;
  onMove: (c: ContainerInfo) => void;
  onRecreate: (c: ContainerInfo) => void;
  onRemove: (c: ContainerInfo) => void;
};

export function ContainerCardActions({
  container: c,
  locked,
  webUrl,
  onAction,
  onTerminal,
  onLogs,
  onRename,
  onMove,
  onRecreate,
  onRemove,
}: ContainerCardActionsProps) {
  const { t } = useTranslation('containers');
  const gate = useHostSelfGate(c);
  const isRunning = c.State === 'running';
  const isPaused = c.State === 'paused';

  // The footer used to carry nine different hues, one per button. When
  // everything is coloured, colour stops meaning anything; Remove keeps its
  // tone because there it encodes consequence rather than identity.
  return (
    <CardFooter className="gap-1 px-3 py-2" onClick={(e) => e.stopPropagation()}>
      {isRunning ? (
        <ActionButton
          label={t('actions.stop')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={() => onAction(c.Id, 'stop')}
          disabled={locked}
          icon={<IconPlayerStop className="h-3.5 w-3.5" />}
        />
      ) : isPaused ? (
        <ActionButton
          label={t('actions.resume')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={() => onAction(c.Id, 'unpause')}
          disabled={locked}
          icon={<IconPlayerPlay className="h-3.5 w-3.5" />}
        />
      ) : (
        <ActionButton
          label={t('actions.start')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={() => onAction(c.Id, 'start')}
          disabled={locked}
          icon={<IconPlayerPlay className="h-3.5 w-3.5" />}
        />
      )}
      <ActionButton
        label={t('actions.restart')}
        permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
        onClick={() => onAction(c.Id, 'restart')}
        disabled={locked}
        icon={<IconRotate className="h-3.5 w-3.5" />}
      />
      <ActionButton
        label={t('actions.recreate')}
        permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
        onClick={() => onRecreate(c)}
        disabled={locked}
        icon={<IconBlocks className="h-3.5 w-3.5" />}
      />
      <ActionButton
        label={t('actions.rename')}
        permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
        onClick={() => onRename(c)}
        disabled={locked}
        icon={<IconTag className="h-3.5 w-3.5" />}
      />
      <ActionButton
        label={t('actions.move')}
        permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
        onClick={() => onMove(c)}
        disabled={locked}
        icon={<IconArrowsExchange className="h-3.5 w-3.5" />}
      />
      {isRunning && (
        <ActionButton
          label={t('actions.terminal')}
          permission={gate.exec(PERM.TERMINAL_ACCESS)}
          onClick={() => onTerminal(c)}
          disabled={locked}
          icon={<IconTerminal2 className="h-3.5 w-3.5" />}
        />
      )}
      <ActionButton
        label={t('actions.logs')}
        onClick={() => onLogs(c)}
        icon={<IconFileText className="h-3.5 w-3.5" />}
      />
      {webUrl && (
        <ActionButton
          label={t('actions.openWebsite')}
          onClick={() => window.open(webUrl, '_blank', 'noopener,noreferrer')}
          icon={<IconExternalLink className="h-3.5 w-3.5" />}
        />
      )}
      <div className="flex-1" />
      <ActionButton
        label={t('actions.remove')}
        permission={gate.lifecycle(PERM.CONTAINERS_DELETE)}
        onClick={() => onRemove(c)}
        disabled={locked}
        icon={<IconTrash className="h-3.5 w-3.5 text-destructive" />}
      />
    </CardFooter>
  );
}
