// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Command } from 'cmdk';
import { EnvTag, GROUP_CLASS, ITEM_CLASS } from './GlobalSearchShared';
import type { PluginSearchGroup } from './useGlobalSearchPluginData';

type PluginGroupsProps = {
  groups: PluginSearchGroup[];
  select: (path: string, envId?: string) => void;
};

export function GlobalSearchPluginGroups({ groups, select }: PluginGroupsProps) {
  const { t } = useTranslation('common');
  return (
    <>
      {groups.map((group) => (
        <Command.Group
          key={group.id}
          heading={t('globalSearch.pluginGroup', { label: group.label, count: group.items.length })}
          className={GROUP_CLASS}
        >
          {group.items.map((item) => (
            <Command.Item
              key={item.key}
              value={`${group.id} ${item.key} ${item.label} ${item.detail}`}
              onSelect={() => select(item.to, item.envId)}
              className={ITEM_CLASS}
            >
              <group.icon className="h-4 w-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 truncate font-medium">{item.label}</span>
              {item.detail && <span className="min-w-0 truncate text-xs text-muted-foreground">{item.detail}</span>}
              {item.envName && <EnvTag name={item.envName} />}
            </Command.Item>
          ))}
        </Command.Group>
      ))}
    </>
  );
}
