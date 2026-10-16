// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useContainers } from '@resources/hooks/useContainers';

export const ALL_CONTAINERS = '*';

/** Splits a stored `containerMatch` into individual names/patterns. `*` or empty means "all". */
export function parseContainerMatch(value: string | undefined): string[] {
  if (!value || value.trim() === ALL_CONTAINERS) return [];
  return value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}

/** Joins names back into the stored form; an empty list becomes `*`. */
export function serializeContainerMatch(names: string[]): string {
  return names.length === 0 ? ALL_CONTAINERS : names.join(',');
}

export type ContainerRow = { name: string; image: string; state: string };

// usePolicyContainerDraft owns the editable selection behind the policy
// container picker: the "all" switch, the picked names, the search box,
// and the derived row lists. `value` is re-parsed every time `open` flips
// to true so a cancelled edit never leaks into the next session.
export function usePolicyContainerDraft(isOpen: boolean, value: string) {
  const { data: containers = [], isLoading } = useContainers();
  const [all, setAll] = useState(true);
  const [draft, setDraft] = useState<string[]>([]);
  const [search, setSearch] = useState('');

  const [seeded, setSeeded] = useState<{ isOpen: boolean; value: string | undefined }>({
    isOpen: false,
    value: undefined,
  });
  if (seeded.isOpen !== isOpen || seeded.value !== value) {
    setSeeded({ isOpen, value });
    if (isOpen) {
      const names = parseContainerMatch(value);
      setAll(names.length === 0);
      setDraft(names);
      setSearch('');
    }
  }

  const rows = useMemo<ContainerRow[]>(
    () =>
      containers.map((c) => ({
        name: c.Names?.[0]?.replace(/^\//, '') ?? c.Id.slice(0, 12),
        image: c.Image,
        state: c.State,
      })),
    [containers],
  );

  const knownNames = useMemo(() => new Set(rows.map((r) => r.name)), [rows]);
  const orphans = draft.filter((itemName) => !knownNames.has(itemName));

  const query = search.trim().toLowerCase();
  const filtered = query
    ? rows.filter((r) => r.name.toLowerCase().includes(query) || r.image.toLowerCase().includes(query))
    : rows;
  const filteredOrphans = query ? orphans.filter((n) => n.toLowerCase().includes(query)) : orphans;

  const candidate = search.trim();
  const canAddPattern = candidate !== '' && !knownNames.has(candidate) && !draft.includes(candidate);

  const toggle = (itemName: string, checked: boolean) => {
    setDraft((prev) => (checked ? [...prev, itemName] : prev.filter((n) => n !== itemName)));
  };

  const addPattern = () => {
    if (!canAddPattern) return;
    setDraft((prev) => [...prev, candidate]);
    setSearch('');
  };

  const canSave = all || draft.length > 0;
  const result = all ? ALL_CONTAINERS : serializeContainerMatch(draft);

  return {
    isLoading,
    all,
    setAll,
    draft,
    search,
    setSearch,
    rows,
    orphans,
    filtered,
    filteredOrphans,
    candidate,
    canAddPattern,
    toggle,
    addPattern,
    canSave,
    result,
  };
}
