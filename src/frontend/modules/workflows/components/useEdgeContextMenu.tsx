// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { IconZoomScan, IconZoomCancel, IconTrash } from '@tabler/icons-react';
import { useCanvasStore } from '../stores/canvasStore';
import type { ContextMenuItem } from './WorkflowContextMenu';

export function useEdgeContextMenu() {
  const { t } = useTranslation('common');
  const selectEdge = useCanvasStore((s) => s.selectEdge);
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; edgeId: string } | null>(null);

  const onEdgeContextMenu = useCallback((edgeId: string, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    selectEdge(edgeId);
    setContextMenu({ x: e.clientX, y: e.clientY, edgeId });
  }, [selectEdge]);

  const onContextMenuSelect = useCallback((key: string) => {
    if (!contextMenu) return;
    const { edgeId } = contextMenu;
    const store = useCanvasStore.getState();

    switch (key) {
      case 'add-sniffer':
        store.updateEdgeSniffer(edgeId, { name: 'Sniffer' });
        break;
      case 'remove-sniffer':
        store.updateEdgeSniffer(edgeId, null);
        break;
      case 'delete-edge':
        store.removeEdge(edgeId);
        break;
    }
    setContextMenu(null);
  }, [contextMenu]);

  const edgeContextMenuItems = useCallback((): ContextMenuItem[] => {
    if (!contextMenu) return [];
    const edge = useCanvasStore.getState().edges.find((e) => e.id === contextMenu.edgeId);
    if (!edge) return [];

    return [
      edge.sniffer
        ? { key: 'remove-sniffer', label: t('workflows.contextRemoveSniffer'), icon: <IconZoomCancel className="size-3.5" /> }
        : { key: 'add-sniffer', label: t('workflows.contextAddSniffer'), icon: <IconZoomScan className="size-3.5" /> },
      { key: 'separator', label: '', separator: true },
      { key: 'delete-edge', label: t('workflows.contextDeleteConnection'), icon: <IconTrash className="size-3.5" />, danger: true, shortcut: 'Del' },
    ];
  }, [contextMenu, t]);

  return {
    contextMenu,
    setContextMenu,
    edgeContextMenuItems,
    onContextMenuSelect,
    onEdgeContextMenu,
  };
}

