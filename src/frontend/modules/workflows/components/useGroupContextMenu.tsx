// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useCallback, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import i18n from '@core/i18n/i18n';
import {
  IconTrash,
  IconBoxMultiple,
  IconEdit,
  IconLock,
  IconLockOpen,
  IconLayoutAlignLeft,
  IconLayoutAlignRight,
  IconLayoutAlignCenter,
  IconLayoutAlignTop,
  IconLayoutAlignBottom,
  IconLayoutAlignMiddle,
  IconLayoutDistributeHorizontal,
  IconLayoutDistributeVertical,
  IconAlignBoxCenterMiddle,
  IconCircleDot,
} from '@tabler/icons-react';
import { useCanvasStore } from '../stores/canvasStore';
import type { ContextMenuItem } from './WorkflowContextMenu';

const GROUP_COLORS = ['#3b82f6', '#8b5cf6', '#f59e0b', '#10b981', '#ef4444', '#ec4899', '#06b6d4', '#f97316'];

export type GroupDialogState = { nodeIds: string[] } | null;
export type EditGroupDialogState = { groupId: string; name: string; color: string } | null;

export function useGroupContextMenu() {
  const { t } = useTranslation('common');
  const createGroup = useCanvasStore((s) => s.createGroup);
  const updateGroup = useCanvasStore((s) => s.updateGroup);
  const toggleGroupBlocked = useCanvasStore((s) => s.toggleGroupBlocked);
  const removeGroup = useCanvasStore((s) => s.removeGroup);
  const alignNodes = useCanvasStore((s) => s.alignNodes);

  const [canvasContextMenu, setCanvasContextMenu] = useState<{ x: number; y: number; canvasX: number; canvasY: number } | null>(null);
  const [groupContextMenu, setGroupContextMenu] = useState<{ x: number; y: number; groupId: string } | null>(null);
  const [groupDialog, setGroupDialog] = useState<GroupDialogState>(null);
  const [groupName, setGroupName] = useState('');
  const groupInputRef = useRef<HTMLInputElement>(null);
  const [editGroupDialog, setEditGroupDialog] = useState<EditGroupDialogState>(null);
  const editGroupInputRef = useRef<HTMLInputElement>(null);

  // Ctrl+G keyboard shortcut for grouping
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || e.target instanceof HTMLSelectElement) return;
      if ((e.ctrlKey || e.metaKey) && e.key === 'g') {
        e.preventDefault();
        const sel = useCanvasStore.getState().selectedNodeIds;
        if (sel.size >= 2) {
          setGroupDialog({ nodeIds: [...sel] });
          setGroupName('');
          setTimeout(() => groupInputRef.current?.focus(), 0);
        }
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);

  // --- Canvas context menu ---
  const onCanvasContextMenu = useCallback((e: React.MouseEvent, canvasEl: HTMLDivElement | null) => {
    const target = e.target as HTMLElement;
    if (target === canvasEl || target.classList.contains('canvas-grid')) {
      e.preventDefault();
      e.stopPropagation();
      // Calculate canvas-space coordinates for node placement
      const vp = useCanvasStore.getState().viewport;
      const rect = canvasEl?.getBoundingClientRect();
      const canvasX = rect ? (e.clientX - rect.left - vp.x) / vp.zoom : 0;
      const canvasY = rect ? (e.clientY - rect.top - vp.y) / vp.zoom : 0;
      setCanvasContextMenu({ x: e.clientX, y: e.clientY, canvasX, canvasY });
    }
  }, []);

  const onCanvasContextMenuSelect = useCallback((key: string) => {
    if (key === 'place-junction' && canvasContextMenu) {
      useCanvasStore.getState().addNode({
        type: 'action',
        action: 'junction',
        label: i18n.t('nodes:junction.label', { defaultValue: 'Junction' }),
        config: {},
        position: { x: Math.round(canvasContextMenu.canvasX - 10), y: Math.round(canvasContextMenu.canvasY - 10) },
      });
    }
    if (key === 'create-group') {
      const sel = useCanvasStore.getState().selectedNodeIds;
      if (sel.size >= 2) {
        setGroupDialog({ nodeIds: [...sel] });
        setGroupName('');
        setTimeout(() => groupInputRef.current?.focus(), 0);
      }
    }
    if (key.startsWith('align-')) {
      const sel = useCanvasStore.getState().selectedNodeIds;
      if (sel.size >= 2) {
        alignNodes([...sel], key.replace('align-', ''));
      }
    }
    setCanvasContextMenu(null);
  }, [alignNodes, canvasContextMenu]);

  const canvasContextMenuItems = useCallback((): ContextMenuItem[] => {
    const sel = useCanvasStore.getState().selectedNodeIds;
    const items: ContextMenuItem[] = [
      {
        key: 'place-junction',
        label: t('workflows.contextPlaceJunction'),
        icon: <IconCircleDot className="size-3.5" />,
      },
    ];

    if (sel.size >= 2) {
      items.push(
        { key: 'separator-group', label: '', separator: true },
        {
          key: 'create-group',
          label: t('workflows.contextCreateGroup'),
          icon: <IconBoxMultiple className="size-3.5" />,
          shortcut: 'Ctrl+G',
        },
        { key: 'separator-align', label: '', separator: true },
        {
          key: 'align',
          label: t('workflows.contextAlign'),
          icon: <IconAlignBoxCenterMiddle className="size-3.5" />,
          children: [
            { key: 'align-left', label: t('workflows.contextAlignLeft'), icon: <IconLayoutAlignLeft className="size-3.5" /> },
            { key: 'align-center-h', label: t('workflows.contextCenterH'), icon: <IconLayoutAlignCenter className="size-3.5" /> },
            { key: 'align-right', label: t('workflows.contextAlignRight'), icon: <IconLayoutAlignRight className="size-3.5" /> },
            { key: 'align-space-h', label: t('workflows.contextSpaceH'), icon: <IconLayoutDistributeHorizontal className="size-3.5" /> },
            { key: 'separator-v', label: '', separator: true },
            { key: 'align-top', label: t('workflows.contextAlignTop'), icon: <IconLayoutAlignTop className="size-3.5" /> },
            { key: 'align-center-v', label: t('workflows.contextCenterV'), icon: <IconLayoutAlignMiddle className="size-3.5" /> },
            { key: 'align-bottom', label: t('workflows.contextAlignBottom'), icon: <IconLayoutAlignBottom className="size-3.5" /> },
            { key: 'align-space-v', label: t('workflows.contextSpaceV'), icon: <IconLayoutDistributeVertical className="size-3.5" /> },
          ],
        },
      );
    }

    return items;
  }, [t]);

  // --- Group context menu ---
  const onGroupContextMenu = useCallback((groupId: string, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setGroupContextMenu({ x: e.clientX, y: e.clientY, groupId });
  }, []);

  const onGroupContextMenuSelect = useCallback((key: string) => {
    if (!groupContextMenu) return;
    if (key === 'edit-group') {
      const group = useCanvasStore.getState().groups.find((g) => g.id === groupContextMenu.groupId);
      if (group) {
        setEditGroupDialog({ groupId: group.id, name: group.name, color: group.color });
        setTimeout(() => editGroupInputRef.current?.focus(), 0);
      }
    }
    if (key === 'toggle-block-group') {
      toggleGroupBlocked(groupContextMenu.groupId);
    }
    if (key === 'delete-group') {
      removeGroup(groupContextMenu.groupId);
    }
    setGroupContextMenu(null);
  }, [groupContextMenu, removeGroup, toggleGroupBlocked]);

  const groupContextMenuItems = useCallback((): ContextMenuItem[] => {
    if (!groupContextMenu) return [];
    const group = useCanvasStore.getState().groups.find((g) => g.id === groupContextMenu.groupId);
    const isBlocked = group?.blocked ?? false;
    return [
      { key: 'edit-group', label: t('workflows.contextEditGroup'), icon: <IconEdit className="size-3.5" /> },
      { key: 'toggle-block-group', label: isBlocked ? t('workflows.contextUnblockGroup') : t('workflows.contextBlockGroup'), icon: isBlocked ? <IconLockOpen className="size-3.5" /> : <IconLock className="size-3.5" /> },
      { key: 'separator', label: '', separator: true },
      { key: 'delete-group', label: t('workflows.contextUngroup'), icon: <IconTrash className="size-3.5" />, danger: true },
    ];
  }, [groupContextMenu, t]);

  const commitGroup = useCallback(() => {
    if (!groupDialog) return;
    createGroup(groupName.trim() || 'Group', GROUP_COLORS[Math.floor(Math.random() * GROUP_COLORS.length)]!, groupDialog.nodeIds);
    setGroupDialog(null);
    setGroupName('');
  }, [groupDialog, groupName, createGroup]);

  const commitEditGroup = useCallback(() => {
    if (!editGroupDialog) return;
    updateGroup(editGroupDialog.groupId, { name: editGroupDialog.name.trim() || 'Group', color: editGroupDialog.color });
    setEditGroupDialog(null);
  }, [editGroupDialog, updateGroup]);

  return {
    canvasContextMenu, setCanvasContextMenu, canvasContextMenuItems, onCanvasContextMenu, onCanvasContextMenuSelect,
    groupContextMenu, setGroupContextMenu, groupContextMenuItems, onGroupContextMenu, onGroupContextMenuSelect,
    groupDialog, setGroupDialog, groupName, setGroupName, groupInputRef,
    editGroupDialog, setEditGroupDialog, editGroupInputRef, commitGroup, commitEditGroup,
  };
}

