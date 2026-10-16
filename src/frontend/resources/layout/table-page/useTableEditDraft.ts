// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { resolveLayout, type ResolvedLayout } from './table-layout';
import type { TableView } from './useTableView';
import type { TableColumnAlign, TableColumnWidth } from './types';

export function useTableEditDraft(view: TableView, isOpen: boolean) {
  const [draft, setDraft] = useState<ResolvedLayout>(view.resolved);
  const [wasOpen, setWasOpen] = useState(isOpen);
  if (wasOpen !== isOpen) {
    setWasOpen(isOpen);
    if (isOpen) setDraft(view.resolved);
  }

  const locked = useMemo(() => new Set(view.declared.filter((c) => c.locked).map((c) => c.id)), [view.declared]);
  const shown = draft.order.filter((id) => draft.visibility[id] !== false);
  const hidden = view.declared.map((c) => c.id).filter((id) => draft.visibility[id] === false);

  const move = (id: string, delta: -1 | 1) =>
    setDraft((current) => {
      const visible = current.order.filter((c) => current.visibility[c] !== false);
      const target = visible[visible.indexOf(id) + delta];
      if (!target || locked.has(target) || locked.has(id)) return current;
      return moveBefore(current, id, target, delta);
    });

  const moveItemTo = (id: string, targetId: string) =>
    setDraft((current) => {
      if (id === targetId || locked.has(id) || locked.has(targetId)) return current;
      const delta = current.order.indexOf(id) < current.order.indexOf(targetId) ? 1 : -1;
      return moveBefore(current, id, targetId, delta);
    });

  const show = (id: string) =>
    setDraft((current) => {
      const order = current.order.filter((c) => c !== id);
      const lastUnlocked = [...order].reverse().find((c) => !locked.has(c) && current.visibility[c] !== false);
      order.splice(lastUnlocked ? order.indexOf(lastUnlocked) + 1 : 0, 0, id);
      return { ...current, order, visibility: { ...current.visibility, [id]: true } };
    });

  const hide = (id: string) =>
    setDraft((current) =>
      locked.has(id) ? current : { ...current, visibility: { ...current.visibility, [id]: false } },
    );

  const resize = (id: string, size: TableColumnWidth) =>
    setDraft((current) => ({ ...current, sizes: { ...current.sizes, [id]: size } }));

  const align = (id: string, part: 'header' | 'cell', value: TableColumnAlign) =>
    setDraft((current) =>
      part === 'header'
        ? { ...current, headerAlign: { ...current.headerAlign, [id]: value } }
        : { ...current, cellAlign: { ...current.cellAlign, [id]: value } },
    );

  const setSortable = (id: string, value: boolean) =>
    setDraft((current) => ({ ...current, sortable: { ...current.sortable, [id]: value } }));

  const reset = () => setDraft(resolveLayout(view.declared, null));

  const sortCapable = useMemo(() => new Set(view.declared.filter((c) => c.canSort).map((c) => c.id)), [view.declared]);

  return {
    draft,
    shown,
    hidden,
    locked,
    sortCapable,
    move,
    moveTo: moveItemTo,
    show,
    hide,
    resize,
    align,
    setSortable,
    reset,
  };
}

function moveBefore(current: ResolvedLayout, id: string, targetId: string, delta: -1 | 1): ResolvedLayout {
  const order = current.order.filter((c) => c !== id);
  const at = order.indexOf(targetId) + (delta === 1 ? 1 : 0);
  order.splice(at, 0, id);
  return { ...current, order };
}
