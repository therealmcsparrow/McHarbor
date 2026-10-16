// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { LayoutItem } from 'react-grid-layout';
import type { WidgetTypeId } from '../widgets/catalog';
import {
  ALL_ENVIRONMENTS_SCOPE,
  buildDefaultLayouts,
  buildDefaultSnapshot,
  cloneSnapshot,
  genId,
  migratePersistedLayout,
  getWidgetMeta,
  normalizeScope,
  resizeItem,
  snapshotState,
  widgetMinSize,
  widgetSize,
  type DashboardSnapshot,
  type Layouts,
  type WidgetInstance,
} from './dashboard-layout-builders';

export type { WidgetInstance };

type DashboardLayoutState = {
  widgets: WidgetInstance[];
  layouts: Layouts;
  dashboards: Record<string, DashboardSnapshot>;
  environmentScope: string;
  editMode: boolean;
  setEnvironmentScope: (environmentId: string) => void;
  setEditMode: (on: boolean) => void;
  addWidget: (typeId: WidgetTypeId) => void;
  removeWidget: (instanceId: string) => void;
  resizeWidget: (instanceId: string, w: number, h: number) => void;
  updateLayouts: (layouts: Layouts) => void;
  pruneUnavailable: (availableTypeIds: Set<string>) => void;
  resetToDefault: () => void;
  /** Replace this scope's dashboard with a named template's widget list. */
  applyTemplate: (widgets: { typeId: string }[]) => void;
};

function scopedUpdate(
  set: (partial: Partial<DashboardLayoutState>) => void,
  get: () => DashboardLayoutState,
  next: DashboardSnapshot,
) {
  const state = get();
  const scope = normalizeScope(state.environmentScope);
  set({
    ...next,
    dashboards: {
      ...state.dashboards,
      [scope]: cloneSnapshot(next),
    },
  });
}

const defaultDashboard = buildDefaultSnapshot();

export const useDashboardLayoutStore = create<DashboardLayoutState>()(
  persist(
    (set, get) => ({
      widgets: defaultDashboard.widgets,
      layouts: defaultDashboard.layouts,
      dashboards: { [ALL_ENVIRONMENTS_SCOPE]: cloneSnapshot(defaultDashboard) },
      environmentScope: ALL_ENVIRONMENTS_SCOPE,
      editMode: false,

      setEnvironmentScope: (environmentId) => {
        const nextScope = normalizeScope(environmentId);
        const state = get();
        const currentScope = normalizeScope(state.environmentScope);
        if (nextScope === currentScope) {
          return;
        }

        const dashboards = {
          ...state.dashboards,
          [currentScope]: cloneSnapshot(snapshotState(state)),
        };
        const nextSnapshot = dashboards[nextScope] ?? buildDefaultSnapshot();

        set({
          environmentScope: nextScope,
          widgets: [...nextSnapshot.widgets],
          layouts: {
            lg: [...nextSnapshot.layouts.lg],
            md: [...nextSnapshot.layouts.md],
            sm: [...nextSnapshot.layouts.sm],
          },
          dashboards: {
            ...dashboards,
            [nextScope]: cloneSnapshot(nextSnapshot),
          },
        });
      },

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

        scopedUpdate(set, get, {
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
        scopedUpdate(set, get, {
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
        const resize = (items: LayoutItem[], maxCols: number) => resizeItem(items, instanceId, w, h, maxCols);
        scopedUpdate(set, get, {
          widgets: get().widgets,
          layouts: {
            lg: resize(layouts.lg, 12),
            md: resize(layouts.md, 8),
            sm: resize(layouts.sm, 4),
          },
        });
      },

      updateLayouts: (layouts) => scopedUpdate(set, get, { widgets: get().widgets, layouts }),

      pruneUnavailable: (availableTypeIds) => {
        const { widgets, layouts } = get();
        const allowedInstanceIds = new Set(
          widgets.filter((widget) => availableTypeIds.has(widget.typeId)).map((widget) => widget.id),
        );

        scopedUpdate(set, get, {
          widgets: widgets.filter((widget) => allowedInstanceIds.has(widget.id)),
          layouts: {
            lg: layouts.lg.filter((item) => allowedInstanceIds.has(item.i)),
            md: layouts.md.filter((item) => allowedInstanceIds.has(item.i)),
            sm: layouts.sm.filter((item) => allowedInstanceIds.has(item.i)),
          },
        });
      },

      resetToDefault: () => {
        scopedUpdate(set, get, { ...buildDefaultSnapshot() });
        set({ editMode: false });
      },

      /**
       * Replace this scope's dashboard with a named template.
       *
       * `templates.ts` has shipped five of these since it was written and
       * nothing ever applied them — there was no action to call and no UI
       * that offered them, so the file was a well-formed list nobody could
       * use. The dashboard's own template picker now offers them.
       *
       * Template entries that name a widget type this build does not have
       * (a plugin widget from an uninstalled plugin) are skipped rather
       * than producing an empty tile.
       */
      applyTemplate: (widgets) => {
        const instances: WidgetInstance[] = widgets
          .filter((entry) => Boolean(getWidgetMeta(entry.typeId)))
          .map((entry) => ({ id: genId(), typeId: entry.typeId }));

        if (instances.length === 0) return;

        scopedUpdate(set, get, {
          widgets: instances,
          layouts: buildDefaultLayouts(instances),
        });
        set({ editMode: false });
      },
    }),
    {
      name: 'mcharbor-dashboard-layout',
      version: 2,
      partialize: (state) => ({
        widgets: state.widgets,
        layouts: state.layouts,
        dashboards: state.dashboards,
        environmentScope: state.environmentScope,
      }),
      migrate: migratePersistedLayout,
    },
  ),
);
