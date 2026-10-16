// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Command } from 'cmdk';
import { IconCommand } from '@tabler/icons-react';
import { GROUP_CLASS, ITEM_CLASS } from './GlobalSearchShared';
import type { GlobalSearchResults, RegisteredCommand } from './useGlobalSearchResults';

type CommandGroupsProps = {
  results: GlobalSearchResults;
  query: string;
  select: (path: string) => void;
  selectRegisteredCommand: (command: RegisteredCommand) => void;
};

export function GlobalSearchCommandGroups({ results, query, select, selectRegisteredCommand }: CommandGroupsProps) {
  const { t } = useTranslation('common');
  const { registeredCommandGroups, searchLinks, filteredNavCommands } = results;
  return (
    <>
      {registeredCommandGroups.map((group) => (
        <Command.Group key={group.id} heading={group.label} className={GROUP_CLASS}>
          {group.commands.map((command) => {
            const Icon = command.icon ?? IconCommand;
            return (
              <Command.Item
                key={command.id}
                value={`${command.group} ${command.label} ${(command.keywords ?? []).join(' ')}`}
                onSelect={() => selectRegisteredCommand(command)}
                className={ITEM_CLASS}
              >
                <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">{command.label}</span>
                {command.shortcut && (
                  <kbd className="shrink-0 rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                    {command.shortcut}
                  </kbd>
                )}
              </Command.Item>
            );
          })}
        </Command.Group>
      ))}

      {searchLinks.length > 0 && (
        <Command.Group heading={t('globalSearch.searchIn')} className={GROUP_CLASS}>
          {searchLinks.map((link) => {
            const Icon = link.icon;
            return (
              <Command.Item
                key={link.id}
                value={`search ${link.label} ${query}`}
                onSelect={() => select(link.to)}
                className={ITEM_CLASS}
              >
                <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">{t('globalSearch.searchInPage', { page: link.label })}</span>
              </Command.Item>
            );
          })}
        </Command.Group>
      )}

      {filteredNavCommands.length > 0 && (
        <Command.Group heading={t('nav.navigation')} className={GROUP_CLASS}>
          {filteredNavCommands.map((cmd) => {
            const label = t(cmd.labelKey);
            return (
              <Command.Item key={cmd.to} value={`nav ${label}`} onSelect={() => select(cmd.to)} className={ITEM_CLASS}>
                <cmd.icon className="h-4 w-4 shrink-0 text-muted-foreground" />
                {label}
              </Command.Item>
            );
          })}
        </Command.Group>
      )}
    </>
  );
}
