// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconTrash,
  IconPlayerTrackNext,
  IconPlayerPause,
  IconPlayerPlay,
  IconBan,
  IconCheck,
} from '@tabler/icons-react';
import { useCanvasStore } from '../stores/canvasStore';
import type { ContextMenuItem } from './WorkflowContextMenu';

export function useNodeContextMenu(onExecute?: (triggerNodeId: string) => void) {
  const { t } = useTranslation('common');
  const selectNode = useCanvasStore((s) => s.selectNode);
  const togglePortBlocked = useCanvasStore((s) => s.togglePortBlocked);

  const [nodeContextMenu, setNodeContextMenu] = useState<{ x: number; y: number; nodeId: string } | null>(null);
  const [portContextMenu, setPortContextMenu] = useState<{ x: number; y: number; nodeId: string; portKey: string } | null>(null);

  const onNodeContextMenu = useCallback((nodeId: string, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    selectNode(nodeId);
    setNodeContextMenu({ x: e.clientX, y: e.clientY, nodeId });
  }, [selectNode]);

  const onNodeContextMenuSelect = useCallback((key: string) => {
    if (!nodeContextMenu) return;
    const store = useCanvasStore.getState();
    const node = store.nodes.find((n) => n.id === nodeContextMenu.nodeId);
    if (!node) return;

    switch (key) {
      case 'skip':
        store.updateNodeSkip(node.id, !node.skip);
        break;
      case 'toggle-disabled':
        store.updateNodeDisabled(node.id, !node.disabled);
        break;
      case 'delete':
        store.removeNode(node.id);
        break;
    }
    setNodeContextMenu(null);
  }, [nodeContextMenu]);

  const nodeContextMenuItems = useCallback((): ContextMenuItem[] => {
    if (!nodeContextMenu) return [];
    const node = useCanvasStore.getState().nodes.find((n) => n.id === nodeContextMenu.nodeId);
    if (!node) return [];

    return [
      {
        key: 'skip',
        label: node.skip ? t('workflows.contextUnskipNode') : t('workflows.contextSkipNode'),
        icon: <IconPlayerTrackNext className="size-3.5" />,
      },
      {
        key: 'toggle-disabled',
        label: node.disabled ? t('workflows.contextEnableNode') : t('workflows.contextDisableNode'),
        icon: node.disabled
          ? <IconPlayerPlay className="size-3.5" />
          : <IconPlayerPause className="size-3.5" />,
      },
      { key: 'separator', label: '', separator: true },
      {
        key: 'delete',
        label: t('workflows.contextDeleteNode'),
        icon: <IconTrash className="size-3.5" />,
        danger: true,
        shortcut: 'Del',
      },
    ];
  }, [nodeContextMenu, t]);

  const onPortContextMenu = useCallback((nodeId: string, portKey: string, e: React.MouseEvent) => {
    setPortContextMenu({ x: e.clientX, y: e.clientY, nodeId, portKey });
  }, []);

  const onPortContextMenuSelect = useCallback((key: string) => {
    if (!portContextMenu) return;
    if (key === 'toggle-block') {
      togglePortBlocked(portContextMenu.nodeId, portContextMenu.portKey);
    }
    setPortContextMenu(null);
  }, [portContextMenu, togglePortBlocked]);

  const portContextMenuItems = useCallback((): ContextMenuItem[] => {
    if (!portContextMenu) return [];
    const node = useCanvasStore.getState().nodes.find((n) => n.id === portContextMenu.nodeId);
    const isBlocked = node?.blockedPorts?.includes(portContextMenu.portKey) ?? false;

    return [
      {
        key: 'toggle-block',
        label: isBlocked ? t('workflows.contextUnblockPort') : t('workflows.contextBlockPort'),
        icon: isBlocked ? <IconCheck className="size-3.5" /> : <IconBan className="size-3.5" />,
        danger: !isBlocked,
      },
    ];
  }, [portContextMenu, t]);

  const onNodeRun = useCallback((nodeId: string) => {
    onExecute?.(nodeId);
  }, [onExecute]);

  return {
    nodeContextMenu,
    setNodeContextMenu,
    nodeContextMenuItems,
    onNodeContextMenu,
    onNodeContextMenuSelect,
    portContextMenu,
    setPortContextMenu,
    portContextMenuItems,
    onPortContextMenu,
    onPortContextMenuSelect,
    onNodeRun,
  };
}

