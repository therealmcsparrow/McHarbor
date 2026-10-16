// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { LayoutItem } from 'react-grid-layout';
import {
  getWidgetMeta as getBuiltinWidgetMeta,
  type WidgetDefinitionMeta,
  type WidgetTypeId,
} from '../widgets/catalog';
import { useWidgetRegistryStore } from './widget-registry';

// Built-ins come from the static catalog; plugin widgets (`plugin:<slug>:<key>`)
// only exist in the registry store once PluginWidgetBridge has loaded them.
export function getWidgetMeta(typeId: string): WidgetDefinitionMeta | undefined {
  return getBuiltinWidgetMeta(typeId) ?? useWidgetRegistryStore.getState().definitionMap[typeId];
}

export type WidgetInstance = {
  id: string;
  typeId: WidgetTypeId;
};

export type Layouts = { lg: LayoutItem[]; md: LayoutItem[]; sm: LayoutItem[] };

export type DashboardSnapshot = {
  widgets: WidgetInstance[];
  layouts: Layouts;
};

let counter = 0;
export function genId(): string {
  return `w-${Date.now()}-${++counter}`;
}

// The first-install dashboard.
//
// This used to be twelve raw-metric widgets: four counts, four host facts,
// four time-series charts. All readings, no verdict — opening McHarbor told
// you the CPU was at 34% but not whether anything needed you.
//
// Three verdict widgets go on top, and the host facts move below the fold
// where a number that changes twice a year belongs. **No widget is
// dropped**: all twelve originals are still here, in the same sizes, and
// anyone who has already customised their layout keeps it — this is the
// seed for a *new* install only.
export const DEFAULT_WIDGETS: WidgetInstance[] = [
  // Is anything wrong?
  { id: 'default-fleet-health', typeId: 'fleet-health' },
  { id: 'default-needs-attention', typeId: 'needs-attention' },
  { id: 'default-update-queue', typeId: 'update-queue' },

  // What is here? (unchanged)
  { id: 'default-containers', typeId: 'containers' },
  { id: 'default-images', typeId: 'images' },
  { id: 'default-volumes', typeId: 'volumes' },
  { id: 'default-networks', typeId: 'networks' },

  // What is it doing? (unchanged)
  { id: 'default-cpu-chart', typeId: 'cpu-chart' },
  { id: 'default-memory-chart', typeId: 'memory-chart' },
  { id: 'default-network-io-chart', typeId: 'network-io-chart' },
  { id: 'default-disk-io-chart', typeId: 'disk-io-chart' },

  // What is it running on? (unchanged, moved down — these change rarely)
  { id: 'default-cpu-cores', typeId: 'cpu-cores' },
  { id: 'default-total-memory', typeId: 'total-memory' },
  { id: 'default-docker-version', typeId: 'docker-version' },
  { id: 'default-disk-usage', typeId: 'disk-usage' },
];

export const ALL_ENVIRONMENTS_SCOPE = 'all';

export function normalizeScope(environmentId: string): string {
  return environmentId || ALL_ENVIRONMENTS_SCOPE;
}

export function getDefaultWidgets(): WidgetInstance[] {
  return DEFAULT_WIDGETS.filter((widget) => Boolean(getWidgetMeta(widget.typeId)));
}

export function widgetSize(typeId: string): { w: number; h: number } {
  return getWidgetMeta(typeId)?.defaultSize ?? { w: 3, h: 1 };
}

export function widgetMinSize(typeId: string): { w: number; h: number } {
  return getWidgetMeta(typeId)?.minSize ?? { w: 2, h: 1 };
}

export function buildLayout(widgets: WidgetInstance[]): LayoutItem[] {
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

export function scaleLg(items: LayoutItem[], cols: number): LayoutItem[] {
  return items.map((l) => {
    const ratio = cols / 12;
    const w = Math.max(Math.round(l.w * ratio), l.minW ?? 1);
    const x = Math.min(Math.round(l.x * ratio), cols - w);
    return { ...l, w, x };
  });
}

export function buildDefaultLayouts(widgets: WidgetInstance[]): Layouts {
  const lg = buildLayout(widgets);
  return {
    lg,
    md: scaleLg(lg, 8),
    sm: scaleLg(lg, 4),
  };
}

export function buildDefaultSnapshot(): DashboardSnapshot {
  const widgets = getDefaultWidgets();
  return { widgets, layouts: buildDefaultLayouts(widgets) };
}

export function cloneSnapshot(snapshot: DashboardSnapshot): DashboardSnapshot {
  return {
    widgets: [...snapshot.widgets],
    layouts: {
      lg: [...snapshot.layouts.lg],
      md: [...snapshot.layouts.md],
      sm: [...snapshot.layouts.sm],
    },
  };
}

export type PersistedDashboardLayout = {
  widgets?: WidgetInstance[];
  layouts?: Layouts;
  dashboards?: Record<string, DashboardSnapshot>;
  environmentScope?: string;
};

export function migratePersistedLayout(persisted: unknown): PersistedDashboardLayout {
  const state = persisted as PersistedDashboardLayout | undefined;
  if (!state) {
    return {};
  }

  const scope = normalizeScope(state.environmentScope ?? '');
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

export function resizeItem(
  items: LayoutItem[],
  instanceId: string,
  w: number,
  h: number,
  maxCols: number,
): LayoutItem[] {
  return items.map((l) => {
    if (l.i !== instanceId) return l;
    const newW = Math.max(l.minW ?? 1, Math.min(w, maxCols));
    const newH = Math.max(l.minH ?? 1, h);
    const newX = Math.min(l.x, maxCols - newW);
    return { ...l, w: newW, h: newH, x: Math.max(0, newX) };
  });
}

export function snapshotState(state: DashboardSnapshot): DashboardSnapshot {
  return {
    widgets: state.widgets,
    layouts: state.layouts,
  };
}
