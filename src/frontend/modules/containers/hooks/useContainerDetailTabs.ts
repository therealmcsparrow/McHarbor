// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useCallback, useEffect, useMemo } from 'react';
import { useSearchParams } from 'react-router';
import { isPluginTabId, usePluginPageTabs } from '@core/plugins/usePluginPageTabs';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import type { ContainerInspect } from '@core/types/docker';
import { detailTabIDs, isDetailTabId, type DetailTabId } from '../components/ContainerDetailTabs';

const stoppedRuntimeTabs = new Set<DetailTabId>(['terminal', 'exec', 'files', 'processes', 'services']);

const imageDerivedTabs = new Set<DetailTabId>(['updates', 'history', 'changes', 'diff']);

function defaultDetailTabForContainer(container: ContainerInspect | undefined): DetailTabId {
  const state = container?.State?.Status;
  if (!container || state === 'running' || state === 'paused') {
    return 'overview';
  }
  if (container.State?.Error || (container.State?.ExitCode ?? 0) !== 0) {
    return 'logs';
  }
  return 'inspect';
}

function readStoredDetailTab(storageKey: string): DetailTabId | null {
  try {
    const stored = window.localStorage.getItem(storageKey);
    return isDetailTabId(stored) ? stored : null;
  } catch {
    return null;
  }
}

function persistDetailTab(storageKey: string, tab: DetailTabId) {
  try {
    window.localStorage.setItem(storageKey, tab);
  } catch {
    // URL state remains authoritative when localStorage is unavailable.
  }
}

/** Resolves the active built-in or plugin tab from `?tab=`, storage and the container's state. */
export function useContainerDetailTabs(
  id: string | undefined,
  container: ContainerInspect | undefined,
  isRunning: boolean,
  isPaused: boolean,
) {
  const pluginTabs = usePluginPageTabs('container');
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedTab = searchParams.get('tab');
  const tabStorageKey = `mcharbor-tabs:container:${id ?? 'unknown'}`;
  const explicitTab = isDetailTabId(requestedTab) ? requestedTab : null;
  const hasExplicitTab = Boolean(explicitTab);
  const storedTab = readStoredDetailTab(tabStorageKey);
  const activeTab: DetailTabId = explicitTab ?? storedTab ?? defaultDetailTabForContainer(container);

  // A plugin tab travels in the same `?tab=` parameter. It is honoured only
  // while the plugin still declares it, so a stale link opens the built-in
  // view rather than an empty panel.
  const requestedPluginTab = isPluginTabId(requestedTab) ? requestedTab : null;
  const pluginTab =
    requestedPluginTab && pluginTabs.some((tab) => tab.id === requestedPluginTab) ? requestedPluginTab : null;
  const showBuiltIn = pluginTab === null;

  useEffect(() => {
    // The built-in tab is what gets remembered: reopening the page on a
    // plugin's tab would be a surprise if that plugin were later removed.
    persistDetailTab(tabStorageKey, activeTab);
  }, [activeTab, tabStorageKey]);
  // A tab the user cannot read is a dead end rather than an informative
  // disabled control, so it is dropped from the strip entirely — the same
  // reasoning the sidebar applies to navigation.
  const canViewBackups = usePermission(PERM.BACKUPS_VIEW);
  const visibleTabs = useMemo(() => {
    const hasImage = Boolean(container?.Image || container?.Config?.Image);
    const tabs = new Set<DetailTabId>(detailTabIDs);
    if (!isRunning && !isPaused) {
      stoppedRuntimeTabs.forEach((tab) => tabs.delete(tab));
    }
    if (!hasImage) {
      imageDerivedTabs.forEach((tab) => tabs.delete(tab));
    }
    if (!canViewBackups) {
      tabs.delete('backup-settings');
      tabs.delete('backup-schedules');
      tabs.delete('backups');
    }
    if (hasExplicitTab) tabs.add(activeTab);
    return detailTabIDs.filter((tab) => tabs.has(tab));
  }, [activeTab, canViewBackups, container?.Config?.Image, container?.Image, hasExplicitTab, isPaused, isRunning]);

  const setPluginTab = useCallback(
    (tab: string) => {
      setSearchParams(
        (current) => {
          current.set('tab', tab);
          return current;
        },
        { preventScrollReset: true },
      );
    },
    [setSearchParams],
  );

  const setActiveTab = useCallback(
    (tab: DetailTabId) => {
      persistDetailTab(tabStorageKey, tab);
      setSearchParams(
        (current) => {
          if (tab === 'overview') current.delete('tab');
          else current.set('tab', tab);
          return current;
        },
        { preventScrollReset: true },
      );
    },
    [setSearchParams, tabStorageKey],
  );

  return {
    pluginTabs,
    activeTab,
    setActiveTab,
    pluginTab,
    setPluginTab,
    showBuiltIn,
    visibleTabs,
  };
}
