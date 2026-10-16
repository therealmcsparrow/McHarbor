// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconArrowsExchange,
  IconPlayerPause,
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconSkull,
  IconTag,
} from '@tabler/icons-react';
import { useCommandRegistry, type CommandEntry } from '@resources/hooks/useCommandRegistry';
import { useTabCommands } from '@resources/hooks/useTabCommands';
import { detailTabIcons, detailTabIDs, type DetailTabId } from '../components/ContainerDetailTabs';

type ContainerDetailCommandsOptions = {
  containerId: string;
  containerName: string;
  isRunning: boolean;
  isPaused: boolean;
  canStart: boolean;
  canRestart: boolean;
  canMove: boolean;
  actionsLocked: boolean;
  onTabChange: (tab: DetailTabId) => void;
  onAction: (action: string) => void;
  onStop: () => void;
  onKill: () => void;
  onMove: () => void;
  onRename: () => void;
};

export function useContainerDetailCommands({
  containerId,
  containerName,
  isRunning,
  isPaused,
  canStart,
  canRestart,
  canMove,
  actionsLocked,
  onTabChange,
  onAction,
  onStop,
  onKill,
  onMove,
  onRename,
}: ContainerDetailCommandsOptions) {
  const { t } = useTranslation('containers');
  const tabCommands = useMemo(() => {
    if (!containerId) return [];
    return detailTabIDs.map((tab) => ({
      id: tab,
      label: t(`detail.${tab}`),
      icon: detailTabIcons[tab],
      keywords: ['container', containerName, tab],
    }));
  }, [containerId, containerName, t]);

  useTabCommands({
    pageId: `container:${containerId}`,
    pageLabel: containerName,
    tabs: tabCommands,
    onTabChange,
    keywords: ['container', containerName],
  });

  const commands = useMemo<CommandEntry[]>(() => {
    if (!containerId) return [];
    if (actionsLocked) return [];

    const actionCommands: CommandEntry[] = [
      {
        id: `container:${containerId}:action:rename`,
        label: `${t('actions.rename')} ${containerName}`,
        icon: IconTag,
        group: 'actions',
        to: '',
        action: onRename,
        keywords: ['container', containerName, 'rename', 'name'],
      },
    ];

    if (canMove) {
      actionCommands.push({
        id: `container:${containerId}:action:move`,
        label: `${t('actions.move')} ${containerName}`,
        icon: IconArrowsExchange,
        group: 'actions',
        to: '',
        action: onMove,
        keywords: ['container', containerName, 'move', 'environment'],
      });
    }

    if (canRestart) {
      actionCommands.unshift({
        id: `container:${containerId}:action:restart`,
        label: `${t('actions.restart')} ${containerName}`,
        icon: IconRotate,
        group: 'actions',
        to: '',
        action: () => onAction('restart'),
        keywords: ['container', containerName, 'restart', 'reboot'],
        priority: 30,
      });
    }

    if (isRunning) {
      actionCommands.push(
        {
          id: `container:${containerId}:action:stop`,
          label: `${t('actions.stop')} ${containerName}`,
          icon: IconPlayerStop,
          group: 'actions',
          to: '',
          action: onStop,
          keywords: ['container', containerName, 'stop'],
          priority: 40,
        },
        {
          id: `container:${containerId}:action:pause`,
          label: `${t('actions.pause')} ${containerName}`,
          icon: IconPlayerPause,
          group: 'actions',
          to: '',
          action: () => onAction('pause'),
          keywords: ['container', containerName, 'pause'],
        },
        {
          id: `container:${containerId}:action:kill`,
          label: `${t('actions.kill')} ${containerName}`,
          icon: IconSkull,
          group: 'actions',
          to: '',
          action: onKill,
          keywords: ['container', containerName, 'kill', 'terminate'],
        },
      );
    } else if (isPaused) {
      actionCommands.push(
        {
          id: `container:${containerId}:action:resume`,
          label: `${t('actions.resume')} ${containerName}`,
          icon: IconPlayerPlay,
          group: 'actions',
          to: '',
          action: () => onAction('unpause'),
          keywords: ['container', containerName, 'resume'],
          priority: 40,
        },
        {
          id: `container:${containerId}:action:stop`,
          label: `${t('actions.stop')} ${containerName}`,
          icon: IconPlayerStop,
          group: 'actions',
          to: '',
          action: onStop,
          keywords: ['container', containerName, 'stop'],
        },
        {
          id: `container:${containerId}:action:kill`,
          label: `${t('actions.kill')} ${containerName}`,
          icon: IconSkull,
          group: 'actions',
          to: '',
          action: onKill,
          keywords: ['container', containerName, 'kill', 'terminate'],
        },
      );
    } else if (canStart) {
      actionCommands.push({
        id: `container:${containerId}:action:start`,
        label: `${t('actions.start')} ${containerName}`,
        icon: IconPlayerPlay,
        group: 'actions',
        to: '',
        action: () => onAction('start'),
        keywords: ['container', containerName, 'start'],
        priority: 40,
      });
    }

    return actionCommands;
  }, [
    actionsLocked,
    canRestart,
    canStart,
    canMove,
    containerId,
    containerName,
    isPaused,
    isRunning,
    onAction,
    onKill,
    onMove,
    onRename,
    onStop,
    t,
  ]);

  useCommandRegistry(commands);
}
