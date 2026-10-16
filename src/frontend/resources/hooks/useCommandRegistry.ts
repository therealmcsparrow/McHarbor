// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from 'react';
import type { ComponentType } from 'react';
import { create } from 'zustand';

// ── Registry ─────────────────────────────────────────────────────────
//
// GlobalSearch reads from this registry so modules can register
// their own commands on mount (deep links to specific containers /
// images / stacks, page-specific actions, etc.). The registry is a
// Zustand store so any component can both register and read
// commands without prop-drilling.

export type CommandGroupId = 'navigation' | 'jump' | 'actions' | 'special';

export type CommandEntry = {
  /** Unique identifier for the entry. Lower-cased, e.g. `jump:container:abc`. */
  id: string;
  /** i18n key (without the namespace prefix — the consumer passes
   *  the resolved label directly via `labelFn` or `label`). */
  label: string;
  /** Optional icon component. */
  icon?: ComponentType<{ className?: string }>;
  /** Optional shortcut hint (e.g. `Mod+K`). */
  shortcut?: string;
  /** Group that the entry belongs to. */
  group: CommandGroupId;
  /** Route to navigate to on selection. */
  to: string;
  /** Optional handler that runs instead of navigation. */
  action?: () => void;
  /** Optional keyword list for fuzzy search. */
  keywords?: string[];
  /** Priority — higher entries surface first within the same group. */
  priority?: number;
};

type CommandRegistryState = {
  entries: RegisteredCommand[];
  register: (entry: CommandEntry) => () => void;
  registerMany: (entries: CommandEntry[]) => () => void;
};

type RegisteredCommand = CommandEntry & {
  registrationToken: symbol;
};

export const useCommandRegistryStore = create<CommandRegistryState>((set) => ({
  entries: [],
  register: (entry) => {
    const registered = { ...entry, registrationToken: Symbol(entry.id) };
    set((state) => {
      // Replace any existing entry with the same id so re-registration
      // is idempotent (modules that update on env change).
      const next = state.entries.filter((e) => e.id !== entry.id);
      next.push(registered);
      return { entries: next };
    });
    return () => {
      set((state) => ({
        entries: state.entries.filter((current) => current.id !== entry.id || current !== registered),
      }));
    };
  },
  registerMany: (entries) => {
    const ids = new Set(entries.map((entry) => entry.id));
    const registered = entries.map((entry) => ({
      ...entry,
      registrationToken: Symbol(entry.id),
    }));
    set((state) => ({
      entries: [...state.entries.filter((entry) => !ids.has(entry.id)), ...registered],
    }));
    return () => {
      set((state) => ({
        entries: state.entries.filter((current) => !ids.has(current.id) || !registered.includes(current)),
      }));
    };
  },
}));

// useCommandRegistry registers a single command while the calling
// component is mounted. Cleanup unregisters it.
export function useCommandRegistry(entry: CommandEntry | CommandEntry[]): void {
  const register = useCommandRegistryStore((s) => s.register);
  const registerMany = useCommandRegistryStore((s) => s.registerMany);
  const entries = useMemo(() => (Array.isArray(entry) ? entry : [entry]), [entry]);
  useEffect(() => {
    if (entries.length === 0) return;
    if (entries.length === 1) {
      return register(entries[0]!);
    }
    return registerMany(entries);
  }, [entries, register, registerMany]);
}

// useRegisteredCommands returns the current set of registered
// commands, sorted by priority within each group. GlobalSearch
// subscribes to this and merges its built-in entries.
export function useRegisteredCommands(): CommandEntry[] {
  const entries = useCommandRegistryStore((s) => s.entries);
  return useMemo(() => [...entries].sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0)), [entries]);
}
