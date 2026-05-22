// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { LayoutItem } from 'react-grid-layout';
import { getWidgetMeta, type WidgetTypeId } from '../widgets/catalog';

export type WidgetInstance = {
  id: string;
  typeId: WidgetTypeId;
};

type Layouts = { lg: LayoutItem[]; md: LayoutItem[]; sm: LayoutItem[] };

type DashboardLayoutState = {
  widgets: WidgetInstance[];
  layouts: Layouts;
  editMode: boolean;
  setEditMode: (on: boolean) => void;
  addWidget: (typeId: WidgetTypeId) => void;
  removeWidget: (instanceId: string) => void;
  resizeWidget: (instanceId: string, w: number, h: number) => void;
  updateLayouts: (layouts: Layouts) => void;
  pruneUnavailable: (availableTypeIds: Set<string>) => void;
  resetToDefault: () => void;
};

let counter = 0;
function genId(): string {
  return `w-${Date.now()}-${++counter}`;
}

const DEFAULT_WIDGETS: WidgetInstance[] = [
  { id: 'default-containers', typeId: 'containers' },
  { id: 'default-images', typeId: 'images' },
  { id: 'default-volumes', typeId: 'volumes' },
  { id: 'default-networks', typeId: 'networks' },
  { id: 'default-cpu-cores', typeId: 'cpu-cores' },
  { id: 'default-total-memory', typeId: 'total-memory' },
  { id: 'default-docker-version', typeId: 'docker-version' },
  { id: 'default-disk-usage', typeId: 'disk-usage' },
  { id: 'default-cpu-chart', typeId: 'cpu-chart' },
  { id: 'default-memory-chart', typeId: 'memory-chart' },
  { id: 'default-network-io-chart', typeId: 'network-io-chart' },
  { id: 'default-disk-io-chart', typeId: 'disk-io-chart' },
];

function getDefaultWidgets(): WidgetInstance[] {
  return DEFAULT_WIDGETS.filter((widget) => Boolean(getWidgetMeta(widget.typeId)));
}

function widgetSize(typeId: string): { w: number; h: number } {
  return getWidgetMeta(typeId)?.defaultSize ?? { w: 3, h: 1 };
}

function widgetMinSize(typeId: string): { w: number; h: number } {
  return getWidgetMeta(typeId)?.minSize ?? { w: 2, h: 1 };
}

function buildLayout(widgets: WidgetInstance[]): LayoutItem[] {
  let x = 0;
  let y = 0;
  let rowMaxH = 0;

  return widgets.map((w) => {
    const size = widgetSize(w.typeId);
    const min = widgetMinSize(w.typeId);

    if (x + size.w > 12) {
      x = 0;
      y += rowMaxH;
      rowMaxH = 0;
    }

    const item: LayoutItem = {
      i: w.id,
      x,
      y,
      w: size.w,
      h: size.h,
      minW: min.w,
      minH: min.h,
    };

    x += size.w;
    rowMaxH = Math.max(rowMaxH, size.h);
    return item;
  });
}

function scaleLg(items: LayoutItem[], cols: number): LayoutItem[] {
  return items.map((l) => {
    const ratio = cols / 12;
    const w = Math.max(Math.round(l.w * ratio), l.minW ?? 1);
    const x = Math.min(Math.round(l.x * ratio), cols - w);
    return { ...l, w, x };
  });
}

function buildDefaultLayouts(widgets: WidgetInstance[]): Layouts {
  const lg = buildLayout(widgets);
  return {
    lg,
    md: scaleLg(lg, 8),
    sm: scaleLg(lg, 4),
  };
}

export const useDashboardLayoutStore = create<DashboardLayoutState>()(
  persist(
    (set, get) => ({
      widgets: getDefaultWidgets(),
      layouts: buildDefaultLayouts(getDefaultWidgets()),
      editMode: false,

      setEditMode: (on) => set({ editMode: on }),

      addWidget: (typeId) => {
        if (!getWidgetMeta(typeId)) {
          return;
        }

        const { widgets, layouts } = get();
        const id = genId();
        const instance: WidgetInstance = { id, typeId };
        const newWidgets = [...widgets, instance];

        const size = widgetSize(typeId);
        const min = widgetMinSize(typeId);

        const maxY = layouts.lg.reduce((m, l) => Math.max(m, l.y + l.h), 0);
        const newItem: LayoutItem = {
          i: id,
          x: 0,
          y: maxY,
          w: size.w,
          h: size.h,
          minW: min.w,
          minH: min.h,
        };

        set({
          widgets: newWidgets,
          layouts: {
            lg: [...layouts.lg, newItem],
            md: [...layouts.md, { ...newItem, w: Math.min(newItem.w, 8), x: 0 }],
            sm: [...layouts.sm, { ...newItem, w: Math.min(newItem.w, 4), x: 0 }],
          },
        });
      },

      removeWidget: (instanceId) => {
        const { widgets, layouts } = get();
        set({
          widgets: widgets.filter((w) => w.id !== instanceId),
          layouts: {
            lg: layouts.lg.filter((l) => l.i !== instanceId),
            md: layouts.md.filter((l) => l.i !== instanceId),
            sm: layouts.sm.filter((l) => l.i !== instanceId),
          },
        });
      },

      resizeWidget: (instanceId, w, h) => {
        const { layouts } = get();
        const resize = (items: LayoutItem[], maxCols: number) =>
          items.map((l) => {
            if (l.i !== instanceId) return l;
            const newW = Math.max(l.minW ?? 1, Math.min(w, maxCols));
            const newH = Math.max(l.minH ?? 1, h);
            const newX = Math.min(l.x, maxCols - newW);
            return { ...l, w: newW, h: newH, x: Math.max(0, newX) };
          });
        set({
          layouts: {
            lg: resize(layouts.lg, 12),
            md: resize(layouts.md, 8),
            sm: resize(layouts.sm, 4),
          },
        });
      },

      updateLayouts: (layouts) => set({ layouts }),

      pruneUnavailable: (availableTypeIds) => {
        const { widgets, layouts } = get();
        const allowedInstanceIds = new Set(
          widgets
            .filter((widget) => availableTypeIds.has(widget.typeId))
            .map((widget) => widget.id),
        );

        set({
          widgets: widgets.filter((widget) => allowedInstanceIds.has(widget.id)),
          layouts: {
            lg: layouts.lg.filter((item) => allowedInstanceIds.has(item.i)),
            md: layouts.md.filter((item) => allowedInstanceIds.has(item.i)),
            sm: layouts.sm.filter((item) => allowedInstanceIds.has(item.i)),
          },
        });
      },

      resetToDefault: () => {
        const defaults = getDefaultWidgets();
        set({
          widgets: defaults,
          layouts: buildDefaultLayouts(defaults),
          editMode: false,
        });
      },
    }),
    { name: 'mcharbor-dashboard-layout' }
  )
);
