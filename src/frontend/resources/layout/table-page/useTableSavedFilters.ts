// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useEffectEvent, useState } from 'react';
import { stableStringify } from './table-layout';
import {
  useCreateFilter,
  useDeleteFilter,
  useTableViewQuery,
  useUpdateFilter,
  type SavedFilter,
} from './useTableViewApi';
import type { TablePageSavedFilters } from './types';

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export type SavedFiltersController = {
  available: boolean;
  filters: SavedFilter[];
  activeId: string;
  modified: boolean;
  pristine: boolean;
  canClear: boolean;
  apply: (id: string) => void;
  create: (name: string, isDefault: boolean) => Promise<void>;
  update: (id: string) => void;
  rename: (id: string, name: string) => Promise<void>;
  setDefault: (id: string, isDefault: boolean) => void;
  remove: (id: string) => void;
  clear: () => void;
};

export function useTableSavedFilters(
  tableId: string,
  config: TablePageSavedFilters | undefined,
): SavedFiltersController {
  const query = useTableViewQuery(tableId);
  const createMutation = useCreateFilter(tableId);
  const updateMutation = useUpdateFilter(tableId);
  const deleteMutation = useDeleteFilter(tableId);
  const [activeId, setActiveId] = useState('');
  const [defaultAppliedFor, setDefaultAppliedFor] = useState<string | null>(null);
  const filters = query.data?.filters ?? [];

  const apply = (id: string) => {
    setActiveId(id);
    const filter = filters.find((entry) => entry.id === id);
    if (filter && config && isPlainObject(filter.state)) config.onRestore(filter.state);
  };

  const restore = useEffectEvent((state: Record<string, unknown>) => config?.onRestore(state));
  const [restoreState, setRestoreState] = useState<Record<string, unknown> | null>(null);

  if (query.isSuccess && config && defaultAppliedFor !== tableId) {
    setDefaultAppliedFor(tableId);
    const initial = config.pristine === false ? undefined : query.data.filters.find((entry) => entry.isDefault);
    if (initial && isPlainObject(initial.state)) {
      setActiveId(initial.id);
      setRestoreState(initial.state);
    }
  }

  useEffect(() => {
    if (restoreState) restore(restoreState);
  }, [restoreState]);

  const active = filters.find((entry) => entry.id === activeId);
  const modified = Boolean(active && config && stableStringify(active.state) !== stableStringify(config.state));

  return {
    available: !query.isError,
    filters,
    activeId,
    modified,
    pristine: config?.pristine ?? true,
    canClear: Boolean(config?.defaults),
    apply,
    create: async (itemName, isDefault) => {
      if (!config) return;
      const created = await createMutation.mutateAsync({ name: itemName, state: config.state, isDefault });
      setActiveId(created.id);
    },
    update: (id) => {
      if (config) updateMutation.mutate({ id, state: config.state });
    },
    rename: async (id, itemName) => {
      await updateMutation.mutateAsync({ id, name: itemName });
    },
    setDefault: (id, isDefault) => updateMutation.mutate({ id, isDefault }),
    remove: (id) => {
      deleteMutation.mutate(id);
      if (id === activeId) setActiveId('');
    },
    clear: () => {
      if (config?.defaults) config.onRestore(config.defaults);
      setActiveId('');
    },
  };
}
