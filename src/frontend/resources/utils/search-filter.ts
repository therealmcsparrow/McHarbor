// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type SearchMode = 'contains' | 'exact' | 'regex';

export type SearchPreset<TState> = {
  id: string;
  label: string;
  state: TState;
};

type SearchMatcherResult = {
  error: string | null;
  matches: (value: string) => boolean;
};

function escapeRegExpLiteral(value: string): string {
  const regExpWithEscape = RegExp as RegExpConstructor & { escape?: (input: string) => string };
  if (typeof regExpWithEscape.escape === 'function') {
    return regExpWithEscape.escape(value);
  }

  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function normalizeValue(value: string): string {
  return value.trim().toLocaleLowerCase();
}

export function createSearchMatcher(query: string, mode: SearchMode): SearchMatcherResult {
  const trimmed = query.trim();
  if (!trimmed) {
    return { error: null, matches: () => true };
  }

  if (mode === 'regex') {
    try {
      const pattern = new RegExp(trimmed, 'i');
      return {
        error: null,
        matches: (value: string) => pattern.test(value),
      };
    } catch {
      return {
        error: 'invalid',
        matches: () => true,
      };
    }
  }

  if (mode === 'exact') {
    const normalized = normalizeValue(trimmed);
    return {
      error: null,
      matches: (value: string) => normalizeValue(value) === normalized,
    };
  }

  const pattern = new RegExp(escapeRegExpLiteral(trimmed), 'i');
  return {
    error: null,
    matches: (value: string) => pattern.test(value),
  };
}

export function matchesSearchFields(
  query: string,
  mode: SearchMode,
  values: Array<string | null | undefined>,
): { error: string | null; matched: boolean } {
  const matcher = createSearchMatcher(query, mode);
  return {
    error: matcher.error,
    matched: values.some((value) => matcher.matches(value ?? '')),
  };
}

export function loadSearchPresets<TState>(storageKey: string): SearchPreset<TState>[] {
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (!raw) {
      return [];
    }

    const parsed = JSON.parse(raw) as SearchPreset<TState>[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function persistSearchPresets<TState>(storageKey: string, presets: SearchPreset<TState>[]) {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(presets));
  } catch {
    // Ignore localStorage failures and keep the in-memory state usable.
  }
}

export function createSearchPresetId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }

  return `preset-${Date.now()}`;
}
