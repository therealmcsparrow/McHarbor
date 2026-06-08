// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { LayoutItem } from "react-grid-layout";
import { getWidgetMeta, type WidgetTypeId } from "../widgets/catalog";

export type WidgetInstance = {
  id: string;
  typeId: WidgetTypeId;
};

type Layouts = { lg: LayoutItem[]; md: LayoutItem[]; sm: LayoutItem[] };

type DashboardSnapshot = {
  widgets: WidgetInstance[];
  layouts: Layouts;
};

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
};

let counter = 0;
function genId(): string {
  return `w-${Date.now()}-${++counter}`;
}

const DEFAULT_WIDGETS: WidgetInstance[] = [
  { id: "default-containers", typeId: "containers" },
  { id: "default-images", typeId: "images" },
  { id: "default-volumes", typeId: "volumes" },
  { id: "default-networks", typeId: "networks" },
  { id: "default-cpu-cores", typeId: "cpu-cores" },
  { id: "default-total-memory", typeId: "total-memory" },
  { id: "default-docker-version", typeId: "docker-version" },
  { id: "default-disk-usage", typeId: "disk-usage" },
  { id: "default-cpu-chart", typeId: "cpu-chart" },
  { id: "default-memory-chart", typeId: "memory-chart" },
  { id: "default-network-io-chart", typeId: "network-io-chart" },
  { id: "default-disk-io-chart", typeId: "disk-io-chart" },
];

const ALL_ENVIRONMENTS_SCOPE = "all";

function normalizeScope(environmentId: string): string {
  return environmentId || ALL_ENVIRONMENTS_SCOPE;
}

function getDefaultWidgets(): WidgetInstance[] {
  return DEFAULT_WIDGETS.filter((widget) =>
    Boolean(getWidgetMeta(widget.typeId)),
  );
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

function buildDefaultSnapshot(): DashboardSnapshot {
  const widgets = getDefaultWidgets();
  return { widgets, layouts: buildDefaultLayouts(widgets) };
}

function cloneSnapshot(snapshot: DashboardSnapshot): DashboardSnapshot {
  return {
    widgets: [...snapshot.widgets],
    layouts: {
      lg: [...snapshot.layouts.lg],
      md: [...snapshot.layouts.md],
      sm: [...snapshot.layouts.sm],
    },
  };
}

function snapshotState(state: DashboardLayoutState): DashboardSnapshot {
  return {
    widgets: state.widgets,
    layouts: state.layouts,
  };
}

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

type PersistedDashboardLayout = Partial<
  Pick<
    DashboardLayoutState,
    "widgets" | "layouts" | "dashboards" | "environmentScope"
  >
>;

function migratePersistedLayout(persisted: unknown): PersistedDashboardLayout {
  const state = persisted as PersistedDashboardLayout | undefined;
  if (!state) {
    return {};
  }

  const scope = normalizeScope(state.environmentScope ?? "");
  const defaults = buildDefaultSnapshot();
  const widgets = state.widgets ?? defaults.widgets;
  const layouts = state.layouts ?? defaults.layouts;
  const dashboards = state.dashboards ?? {
    [scope]: cloneSnapshot({ widgets, layouts }),
  };

  return {
    widgets,
    layouts,
    dashboards,
    environmentScope: scope,
  };
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
            md: [
              ...layouts.md,
              { ...newItem, w: Math.min(newItem.w, 8), x: 0 },
            ],
            sm: [
              ...layouts.sm,
              { ...newItem, w: Math.min(newItem.w, 4), x: 0 },
            ],
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
        const resize = (items: LayoutItem[], maxCols: number) =>
          items.map((l) => {
            if (l.i !== instanceId) return l;
            const newW = Math.max(l.minW ?? 1, Math.min(w, maxCols));
            const newH = Math.max(l.minH ?? 1, h);
            const newX = Math.min(l.x, maxCols - newW);
            return { ...l, w: newW, h: newH, x: Math.max(0, newX) };
          });
        scopedUpdate(set, get, {
          widgets: get().widgets,
          layouts: {
            lg: resize(layouts.lg, 12),
            md: resize(layouts.md, 8),
            sm: resize(layouts.sm, 4),
          },
        });
      },

      updateLayouts: (layouts) =>
        scopedUpdate(set, get, { widgets: get().widgets, layouts }),

      pruneUnavailable: (availableTypeIds) => {
        const { widgets, layouts } = get();
        const allowedInstanceIds = new Set(
          widgets
            .filter((widget) => availableTypeIds.has(widget.typeId))
            .map((widget) => widget.id),
        );

        scopedUpdate(set, get, {
          widgets: widgets.filter((widget) =>
            allowedInstanceIds.has(widget.id),
          ),
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
    }),
    {
      name: "mcharbor-dashboard-layout",
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
