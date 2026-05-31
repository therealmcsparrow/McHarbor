// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
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
import { ActionButton } from './ActionButton';
import { getContainerWebUrl } from './container-utils';

type ContainerActionsCellProps = {
  container: ContainerInfo;
  onAction: (vars: { id: string; action: string }) => void;
  onTerminal: (c: ContainerInfo) => void;
  onLogs: (c: ContainerInfo) => void;
  onRename: (c: ContainerInfo) => void;
  onMove: (c: ContainerInfo) => void;
  onRemove: (c: ContainerInfo) => void;
};

export function ContainerActionsCell({
  container: c,
  onAction,
  onTerminal,
  onLogs,
  onRename,
  onMove,
  onRemove,
}: ContainerActionsCellProps) {
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const isRunning = c.State === 'running';
  const locked = isProtectedContainer(c);
  const webUrl = isRunning ? getContainerWebUrl(c.Ports) : null;

  return (
    <div className="flex items-center justify-end">
      {locked && (
        <ActionButton
          label={tc('actions.locked')}
          onClick={() => undefined}
          disabled
          icon={<IconLock className="h-3.5 w-3.5 text-muted-foreground" />}
        />
      )}
      {isRunning ? (
        <ActionButton
          label={t('actions.stop')}
          onClick={() => onAction({ id: c.Id, action: 'stop' })}
          disabled={locked}
          icon={<IconPlayerStop className="h-3.5 w-3.5 text-amber-500" />}
        />
      ) : (
        <ActionButton
          label={t('actions.start')}
          onClick={() => onAction({ id: c.Id, action: 'start' })}
          disabled={locked}
          icon={<IconPlayerPlay className="h-3.5 w-3.5 text-emerald-500" />}
        />
      )}
      <ActionButton
        label={t('actions.restart')}
        onClick={() => onAction({ id: c.Id, action: 'restart' })}
        disabled={locked}
        icon={<IconRotate className="h-3.5 w-3.5 text-blue-400" />}
      />
      <ActionButton
        label={t('actions.rename')}
        onClick={() => onRename(c)}
        disabled={locked}
        icon={<IconTag className="h-3.5 w-3.5 text-lime-400" />}
      />
      <ActionButton
        label={t('actions.move')}
        onClick={() => onMove(c)}
        disabled={locked}
        icon={<IconArrowsExchange className="h-3.5 w-3.5 text-orange-400" />}
      />
      {isRunning && (
        <ActionButton
          label={t('actions.terminal')}
          onClick={() => onTerminal(c)}
          disabled={locked}
          icon={<IconTerminal2 className="h-3.5 w-3.5 text-violet-400" />}
        />
      )}
      <ActionButton
        label={t('actions.logs')}
        onClick={() => onLogs(c)}
        icon={<IconFileText className="h-3.5 w-3.5 text-cyan-400" />}
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
        onClick={() => onRemove(c)}
        disabled={locked}
        icon={<IconTrash className="h-3.5 w-3.5 text-destructive" />}
      />
    </div>
  );
}
