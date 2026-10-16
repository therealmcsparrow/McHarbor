// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconActivity, IconBox, IconClipboardList, IconFileText } from '@tabler/icons-react';
import { useVisibleNavItems } from '@resources/hooks/useVisibleNavItems';
import { createSearchMatcher, type SearchMode } from '@resources/utils/search-filter';
import { useRegisteredCommands } from '@resources/hooks/useCommandRegistry';
import { NAV_COMMANDS, containerName, imageLabel } from './globalSearchModel';
import type { GlobalSearchData } from './useGlobalSearchData';
import { usePluginSearchActions } from './useGlobalSearchPluginData';

export function useGlobalSearchResults(query: string, mode: SearchMode, data: GlobalSearchData) {
  const { t } = useTranslation('common');
  const hostCommands = useRegisteredCommands();
  const pluginActions = usePluginSearchActions();
  const registeredCommands = useMemo(
    () => [...hostCommands, ...pluginActions].sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0)),
    [hostCommands, pluginActions],
  );
  const visibleNavCommands = useVisibleNavItems(NAV_COMMANDS);
  const { containers, images, volumes, networks, stacks, pluginGroups } = data;

  const matcher = useMemo(() => createSearchMatcher(query, mode), [mode, query]);
  const filteredNavCommands = useMemo(
    () => visibleNavCommands.filter((cmd) => matcher.matches(t(cmd.labelKey))),
    [matcher, t, visibleNavCommands],
  );
  const filteredRegisteredCommands = useMemo(
    () =>
      registeredCommands.filter((command) =>
        matcher.matches(`${command.label} ${command.group} ${(command.keywords ?? []).join(' ')}`),
      ),
    [matcher, registeredCommands],
  );
  const registeredCommandGroups = useMemo(
    () =>
      [
        {
          id: 'actions',
          label: t('globalSearch.actions', {
            count: filteredRegisteredCommands.filter((command) => command.group === 'actions').length,
          }),
          commands: filteredRegisteredCommands.filter((command) => command.group === 'actions'),
        },
        {
          id: 'jump',
          label: t('globalSearch.jumps', {
            count: filteredRegisteredCommands.filter((command) => command.group === 'jump').length,
          }),
          commands: filteredRegisteredCommands.filter((command) => command.group === 'jump'),
        },
        {
          id: 'other',
          label: t('globalSearch.commands', {
            count: filteredRegisteredCommands.filter(
              (command) => command.group !== 'actions' && command.group !== 'jump',
            ).length,
          }),
          commands: filteredRegisteredCommands.filter(
            (command) => command.group !== 'actions' && command.group !== 'jump',
          ),
        },
      ].filter((group) => group.commands.length > 0),
    [filteredRegisteredCommands, t],
  );
  const filteredContainers = useMemo(
    () => containers.filter((c) => matcher.matches(`container ${containerName(c)} ${c.Image} ${c.State} ${c.envName}`)),
    [containers, matcher],
  );
  const filteredImages = useMemo(
    () => images.filter((img) => matcher.matches(`image ${imageLabel(img)} ${img.envName}`)),
    [images, matcher],
  );
  const filteredVolumes = useMemo(
    () => volumes.filter((vol) => matcher.matches(`volume ${vol.Name} ${vol.Driver} ${vol.envName}`)),
    [matcher, volumes],
  );
  const filteredNetworks = useMemo(
    () => networks.filter((net) => matcher.matches(`network ${net.Name} ${net.Driver} ${net.envName}`)),
    [matcher, networks],
  );
  const filteredStacks = useMemo(
    () =>
      stacks.filter((stack) =>
        matcher.matches(`stack ${stack.name} ${stack.status} ${stack.description ?? ''} ${stack.envName}`),
      ),
    [matcher, stacks],
  );
  const filteredPluginGroups = useMemo(
    () =>
      pluginGroups
        .map((group) => ({
          ...group,
          items: group.items.filter((item) =>
            matcher.matches(`${group.label} ${item.label} ${item.detail} ${item.envName ?? ''}`),
          ),
        }))
        .filter((group) => group.items.length > 0),
    [matcher, pluginGroups],
  );
  const searchLinks = useMemo(() => {
    const trimmed = query.trim();
    if (!trimmed) return [];
    const q = encodeURIComponent(trimmed);
    return [
      { id: 'containers', label: t('nav.containers'), to: `/containers?q=${q}`, icon: IconBox },
      { id: 'activity', label: t('nav.activity'), to: `/activity?q=${q}`, icon: IconActivity },
      { id: 'logs', label: t('nav.logs'), to: `/logs?q=${q}`, icon: IconFileText },
      { id: 'lifecycle', label: t('nav.logging'), to: `/lifecycle?q=${q}`, icon: IconClipboardList },
    ];
  }, [query, t]);

  const isEmpty =
    filteredNavCommands.length === 0 &&
    filteredRegisteredCommands.length === 0 &&
    filteredContainers.length === 0 &&
    filteredImages.length === 0 &&
    filteredVolumes.length === 0 &&
    filteredNetworks.length === 0 &&
    filteredStacks.length === 0 &&
    filteredPluginGroups.length === 0 &&
    searchLinks.length === 0;

  return {
    matcher,
    registeredCommandGroups,
    searchLinks,
    filteredNavCommands,
    filteredContainers,
    filteredImages,
    filteredVolumes,
    filteredNetworks,
    filteredStacks,
    filteredPluginGroups,
    isEmpty,
  };
}

export type GlobalSearchResults = ReturnType<typeof useGlobalSearchResults>;
export type RegisteredCommand = ReturnType<typeof useRegisteredCommands>[number];
