// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Command } from 'cmdk';
import { formatBytes } from '@resources/utils/format';
import { EnvTag, GROUP_CLASS, ITEM_CLASS } from './GlobalSearchShared';
import { CONTAINER_DOT, DEFAULT_DOT, STACK_DOT, containerName, imageLabel } from './globalSearchModel';
import { GlobalSearchPluginGroups } from './GlobalSearchPluginGroups';
import type { GlobalSearchResults } from './useGlobalSearchResults';

type ResourceGroupsProps = {
  results: GlobalSearchResults;
  select: (path: string, envId?: string) => void;
};

const NAME_CLASS = 'min-w-0 truncate font-medium';
const DRIVER_CLASS = 'shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground';

function Dot({ className }: { className: string }) {
  return <span className={`h-2 w-2 shrink-0 rounded-full ${className}`} />;
}

export function GlobalSearchResourceGroups({ results, select }: ResourceGroupsProps) {
  const { t } = useTranslation('common');
  const { filteredContainers, filteredImages, filteredVolumes, filteredNetworks, filteredStacks } = results;
  return (
    <>
      {filteredContainers.length > 0 && (
        <Command.Group
          heading={t('globalSearch.containers', { count: filteredContainers.length })}
          className={GROUP_CLASS}
        >
          {filteredContainers.map((c) => {
            const itemName = containerName(c);
            return (
              <Command.Item
                key={`c-${c.envId}-${c.Id}`}
                value={`container ${itemName} ${c.Image} ${c.State} ${c.envName}`}
                onSelect={() => select(`/containers/${c.Id}?env=${c.envId}`)}
                className={ITEM_CLASS}
              >
                <Dot className={CONTAINER_DOT[c.State] ?? DEFAULT_DOT} />
                <span className={NAME_CLASS}>{itemName}</span>
                <span className="min-w-0 truncate text-xs text-muted-foreground">{c.Image}</span>
                <EnvTag name={c.envName} />
              </Command.Item>
            );
          })}
        </Command.Group>
      )}

      {filteredImages.length > 0 && (
        <Command.Group heading={t('globalSearch.images', { count: filteredImages.length })} className={GROUP_CLASS}>
          {filteredImages.map((img) => {
            const label = imageLabel(img);
            return (
              <Command.Item
                key={`i-${img.envId}-${img.Id}`}
                value={`image ${label} ${img.envName}`}
                onSelect={() => select(`/images/${encodeURIComponent(img.Id)}?env=${img.envId}`)}
                className={ITEM_CLASS}
              >
                <Dot className={img.Containers > 0 ? 'bg-success' : DEFAULT_DOT} />
                <span className={NAME_CLASS}>{label}</span>
                <span className="text-xs text-muted-foreground tabular-nums">{formatBytes(img.Size)}</span>
                <EnvTag name={img.envName} />
              </Command.Item>
            );
          })}
        </Command.Group>
      )}

      {filteredVolumes.length > 0 && (
        <Command.Group heading={t('globalSearch.volumes', { count: filteredVolumes.length })} className={GROUP_CLASS}>
          {filteredVolumes.map((vol) => (
            <Command.Item
              key={`v-${vol.envId}-${vol.Name}`}
              value={`volume ${vol.Name} ${vol.Driver} ${vol.envName}`}
              onSelect={() => select(`/volumes?env=${vol.envId}`)}
              className={ITEM_CLASS}
            >
              <Dot className={vol.RefCount > 0 ? 'bg-success' : DEFAULT_DOT} />
              <span className={NAME_CLASS}>{vol.Name}</span>
              <span className={DRIVER_CLASS}>{vol.Driver}</span>
              <EnvTag name={vol.envName} />
            </Command.Item>
          ))}
        </Command.Group>
      )}

      {filteredNetworks.length > 0 && (
        <Command.Group heading={t('globalSearch.networks', { count: filteredNetworks.length })} className={GROUP_CLASS}>
          {filteredNetworks.map((net) => (
            <Command.Item
              key={`n-${net.envId}-${net.Id}`}
              value={`network ${net.Name} ${net.Driver} ${net.envName}`}
              onSelect={() => select(`/networks/${net.Id}?env=${net.envId}`)}
              className={ITEM_CLASS}
            >
              <Dot className="bg-info" />
              <span className={NAME_CLASS}>{net.Name}</span>
              <span className={DRIVER_CLASS}>{net.Driver}</span>
              <EnvTag name={net.envName} />
            </Command.Item>
          ))}
        </Command.Group>
      )}

      {filteredStacks.length > 0 && (
        <Command.Group heading={t('globalSearch.stacks', { count: filteredStacks.length })} className={GROUP_CLASS}>
          {filteredStacks.map((stack) => (
            <Command.Item
              key={`s-${stack.envId}-${stack.name}`}
              value={`stack ${stack.name} ${stack.status} ${stack.description ?? ''} ${stack.envName}`}
              onSelect={() => select(`/stacks/${encodeURIComponent(stack.name)}?env=${stack.envId}`)}
              className={ITEM_CLASS}
            >
              <Dot className={STACK_DOT[stack.status] ?? DEFAULT_DOT} />
              <span className={NAME_CLASS}>{stack.name}</span>
              <span className="shrink-0 text-xs text-muted-foreground">
                {t('stackOverview.services', { count: stack.services.length })}
              </span>
              <EnvTag name={stack.envName} />
            </Command.Item>
          ))}
        </Command.Group>
      )}

      <GlobalSearchPluginGroups groups={results.filteredPluginGroups} select={select} />
    </>
  );
}
