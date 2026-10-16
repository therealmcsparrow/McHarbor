// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { ConfigSection } from '@resources/hooks/useContainerBackups';
import { ConfigSectionRow } from './ConfigSectionRow';

type ConfigItemTreeProps = {
  sections: ConfigSection[];
  // The live container inspect, already on the page via useContainer.
  // Building the tree from it means no extra endpoint and no second
  // source of truth about what the container currently holds.
  inspect: Record<string, unknown> | undefined;
  selected: string[];
  onChange: (paths: string[]) => void;
};

// resolvePath walks a dotted path into the inspect document.
function resolvePath(root: unknown, path: string): unknown {
  let value: unknown = root;
  for (const segment of path.split('.')) {
    if (typeof value !== 'object' || value === null) return undefined;
    value = (value as Record<string, unknown>)[segment];
  }
  return value;
}

// itemNamesFor lists the addressable entries inside one section, matching
// what the backend's copyJSONItem can actually extract: an array of
// "KEY=VALUE" strings, or an object keyed by name. Anything else is a
// scalar the user selects whole, and yields no children.
function itemNamesFor(inspect: unknown, sectionKey: string): string[] {
  const value = resolvePath(inspect, sectionKey);
  if (Array.isArray(value)) {
    return value
      .filter((entry): entry is string => typeof entry === 'string')
      .map((entry) => {
        const index = entry.indexOf('=');
        return index > 0 ? entry.slice(0, index) : entry;
      });
  }
  if (typeof value === 'object' && value !== null) {
    return Object.keys(value as Record<string, unknown>);
  }
  return [];
}

// The config option's "choose" mode. Sections come from the backend
// catalog so capture and picker cannot drift; the items under each come
// from the live container, so what is offered is what is actually there.
export function ConfigItemTree({ sections, inspect, selected, onChange }: ConfigItemTreeProps) {
  const { t } = useTranslation('containers');

  if (sections.length === 0) {
    return <p className="text-xs text-muted-foreground">{t('backups.configSectionsUnavailable')}</p>;
  }

  return (
    <div className="space-y-2">
      <div className="max-h-64 overflow-auto rounded-lg border border-border bg-background p-2">
        {sections.map((section) => (
          <ConfigSectionRow
            key={section.key}
            section={section}
            items={itemNamesFor(inspect, section.key)}
            selected={selected}
            onChange={onChange}
          />
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        {selected.length === 0
          ? t('backups.treeNothingPicked')
          : t('backups.treePickedCount', { count: selected.length })}
      </p>
    </div>
  );
}
