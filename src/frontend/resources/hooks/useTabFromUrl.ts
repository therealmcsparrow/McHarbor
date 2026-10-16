// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect } from 'react';
import { useSearchParams } from 'react-router';

type UseTabFromUrlOptions<TabId extends string> = {
  param?: string;
  clearDefault?: boolean;
  replace?: boolean;
  aliases?: Partial<Record<string, TabId>>;
  storageKey?: string;
  /**
   * Accepts an id the list cannot contain, for tabs that only exist at
   * runtime — a plugin's, whose id is not knowable when this page is
   * compiled. The list stays the authority for everything it can enumerate.
   */
  accepts?: (value: string) => boolean;
};

function readStoredTab<TabId extends string>(
  storageKey: string | undefined,
  tabIds: readonly TabId[],
  accepts?: (value: string) => boolean,
) {
  if (!storageKey) return null;
  try {
    const stored = window.localStorage.getItem(storageKey);
    if (!stored) return null;
    const known = tabIds.some((tabId) => tabId === stored) || Boolean(accepts?.(stored));
    return known ? (stored as TabId) : null;
  } catch {
    return null;
  }
}

function persistTab(storageKey: string | undefined, tab: string) {
  if (!storageKey) return;
  try {
    window.localStorage.setItem(storageKey, tab);
  } catch {
    // URL state still works when localStorage is unavailable.
  }
}

export function useTabFromUrl<TabId extends string>(
  tabIds: readonly TabId[],
  defaultTab: TabId,
  {
    param = 'tab',
    clearDefault = true,
    replace = false,
    aliases,
    storageKey,
    accepts,
  }: UseTabFromUrlOptions<TabId> = {},
) {
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedTab = searchParams.get(param);
  const normalizedTab = requestedTab ? (aliases?.[requestedTab] ?? requestedTab) : null;
  const urlKnown =
    normalizedTab !== null && (tabIds.some((tabId) => tabId === normalizedTab) || Boolean(accepts?.(normalizedTab)));
  const urlTab = urlKnown ? (normalizedTab as TabId) : null;
  const activeTab = urlTab ?? readStoredTab(storageKey, tabIds, accepts) ?? defaultTab;

  useEffect(() => {
    persistTab(storageKey, activeTab);
  }, [activeTab, storageKey]);

  const setActiveTab = useCallback(
    (nextTab: TabId) => {
      if (!tabIds.some((tabId) => tabId === nextTab) && !accepts?.(nextTab)) return;
      persistTab(storageKey, nextTab);

      setSearchParams(
        (current) => {
          const next = new URLSearchParams(current);
          if (clearDefault && nextTab === defaultTab) {
            next.delete(param);
          } else {
            next.set(param, nextTab);
          }
          return next;
        },
        { preventScrollReset: true, replace },
      );
    },
    [accepts, clearDefault, defaultTab, param, replace, setSearchParams, storageKey, tabIds],
  );

  return [activeTab, setActiveTab] as const;
}
