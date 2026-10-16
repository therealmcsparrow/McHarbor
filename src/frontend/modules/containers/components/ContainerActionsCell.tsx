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
  IconLock,
  IconTag,
  IconArrowsExchange,
} from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import { isProtectedContainer } from '@core/utils/protection';
import { ActionButton } from '@resources/components/ui/ActionButton';
import { PERM } from '@core/auth/permissionKeys';
import { useHostSelfGate } from '../hooks/useHostSelfGate';
import { getContainerWebUrl } from './container-utils';

type ContainerActionsCellProps = {
  container: ContainerInfo;
  onAction: (vars: { id: string; action: string }) => void;
  onTerminal: (c: ContainerInfo) => void;
  onLogs: (c: ContainerInfo) => void;
  onRename: (c: ContainerInfo) => void;
  onMove: (c: ContainerInfo) => void;
  onRecreate: (c: ContainerInfo) => void;
  onRemove: (c: ContainerInfo) => void;
};

export function ContainerActionsCell({
  container: c,
  onAction,
  onTerminal,
  onLogs,
  onRename,
  onMove,
  onRecreate,
  onRemove,
}: ContainerActionsCellProps) {
  const { t } = useTranslation('containers');
  const isRunning = c.State === 'running';
  const locked = isProtectedContainer(c);
  const gate = useHostSelfGate(c);
  const webUrl = isRunning ? getContainerWebUrl(c.Ports) : null;

  return (
    <div className="flex items-center justify-end">
      {locked && (
        <ActionButton
          label={t('actions.locked', { ns: 'common' })}
          onClick={() => undefined}
          disabled
          icon={<IconLock className="h-3.5 w-3.5 text-muted-foreground" />}
        />
      )}
      {isRunning ? (
        <ActionButton
          label={t('actions.stop')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={() => onAction({ id: c.Id, action: 'stop' })}
          disabled={locked}
          icon={<IconPlayerStop className="h-3.5 w-3.5 text-warning" />}
        />
      ) : (
        <ActionButton
          label={t('actions.start')}
          permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
          onClick={() => onAction({ id: c.Id, action: 'start' })}
          disabled={locked}
          icon={<IconPlayerPlay className="h-3.5 w-3.5 text-success" />}
        />
      )}
      <ActionButton
        label={t('actions.restart')}
        permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
        onClick={() => onAction({ id: c.Id, action: 'restart' })}
        disabled={locked}
        icon={<IconRotate className="h-3.5 w-3.5 text-info" />}
      />
      <ActionButton
        label={t('actions.recreate')}
        permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
        onClick={() => onRecreate(c)}
        disabled={locked}
        icon={<IconBlocks className="h-3.5 w-3.5 text-chart-5" />}
      />
      <ActionButton
        label={t('actions.rename')}
        permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
        onClick={() => onRename(c)}
        disabled={locked}
        icon={<IconTag className="h-3.5 w-3.5 text-chart-8" />}
      />
      <ActionButton
        label={t('actions.move')}
        permission={gate.lifecycle(PERM.CONTAINERS_MANAGE)}
        onClick={() => onMove(c)}
        disabled={locked}
        icon={<IconArrowsExchange className="h-3.5 w-3.5 text-chart-2" />}
      />
      {isRunning && (
        <ActionButton
          label={t('actions.terminal')}
          permission={gate.exec(PERM.TERMINAL_ACCESS)}
          onClick={() => onTerminal(c)}
          disabled={locked}
          icon={<IconTerminal2 className="h-3.5 w-3.5 text-chart-3" />}
        />
      )}
      <ActionButton
        label={t('actions.logs')}
        onClick={() => onLogs(c)}
        icon={<IconFileText className="h-3.5 w-3.5 text-chart-1" />}
      />
      {webUrl && (
        <ActionButton
          label={t('actions.openWebsite')}
          onClick={() => window.open(webUrl, '_blank', 'noopener,noreferrer')}
          icon={<IconExternalLink className="h-3.5 w-3.5 text-primary" />}
        />
      )}
      <ActionButton
        label={t('actions.remove')}
        permission={gate.lifecycle(PERM.CONTAINERS_DELETE)}
        onClick={() => onRemove(c)}
        disabled={locked}
        icon={<IconTrash className="h-3.5 w-3.5 text-destructive" />}
      />
    </div>
  );
}
