// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconChevronRight, IconFile, IconFolder, IconFolderOpen } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import type { ConfigSection } from '@resources/hooks/useContainerBackups';

// A section covers its own items, the same way a folder covers its files
// in the path tree.
function coveredBy(path: string, sectionKey: string, selected: string[]) {
  return selected.includes(path) || selected.includes(sectionKey);
}

type ConfigSectionRowProps = {
  section: ConfigSection;
  items: string[];
  selected: string[];
  onChange: (paths: string[]) => void;
};

// One expandable section of the ConfigItemTree: the whole-section tick plus
// the per-item ticks underneath it.
export function ConfigSectionRow({ section, items, selected, onChange }: ConfigSectionRowProps) {
  const { t } = useTranslation('containers');
  const [isOpen, setOpen] = useState(false);
  const wholeSelected = selected.includes(section.key);
  const hasItems = items.length > 0;

  function toggleSection(next: boolean) {
    if (next) {
      // Selecting the whole section makes its individual items
      // redundant, so they are dropped rather than left as noise.
      const prefix = `${section.key}.`;
      onChange([...selected.filter((entry) => entry !== section.key && !entry.startsWith(prefix)), section.key]);
      return;
    }
    onChange(selected.filter((entry) => entry !== section.key));
  }

  function toggleItem(entryName: string, next: boolean) {
    const path = `${section.key}.${entryName}`;
    if (next) {
      onChange([...selected.filter((entry) => entry !== path), path]);
      return;
    }
    onChange(selected.filter((entry) => entry !== path));
  }

  return (
    <div>
      <div className="flex items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-muted/40">
        {hasItems ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={isOpen ? t('backups.treeCollapse') : t('backups.treeExpand')}
            aria-expanded={isOpen}
            onClick={() => setOpen((current) => !current)}
            className="size-5 shrink-0 text-muted-foreground hover:text-foreground"
          >
            {isOpen ? (
              <IconChevronDown className="size-4" aria-hidden="true" />
            ) : (
              <IconChevronRight className="size-4" aria-hidden="true" />
            )}
          </Button>
        ) : (
          <span className="size-5 shrink-0" aria-hidden="true" />
        )}
        {hasItems ? (
          isOpen ? (
            <IconFolderOpen className="size-4 shrink-0 text-warning" aria-hidden="true" />
          ) : (
            <IconFolder className="size-4 shrink-0 text-warning" aria-hidden="true" />
          )
        ) : (
          <IconFile className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        )}
        <Checkbox
          checked={wholeSelected}
          onCheckedChange={(checked) => toggleSection(checked === true)}
          aria-label={section.label}
          className="shrink-0"
        />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm text-foreground">{section.label}</span>
          <span className="block truncate text-xs text-muted-foreground">{section.description}</span>
        </span>
        {hasItems && <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{items.length}</span>}
      </div>

      {hasItems && isOpen && (
        <div>
          {items.map((itemName) => {
            const path = `${section.key}.${itemName}`;
            const covered = coveredBy(path, section.key, selected);
            return (
              <div
                key={path}
                className="flex items-center gap-2 rounded-md px-2 py-1 pl-9 transition-colors hover:bg-muted/40"
              >
                <IconFile className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <Checkbox
                  checked={covered}
                  // Already covered by the whole-section pick, so
                  // unticking it here would be a lie about what gets
                  // captured. Untick the section instead.
                  disabled={wholeSelected}
                  onCheckedChange={(checked) => toggleItem(itemName, checked === true)}
                  aria-label={path}
                  className="shrink-0"
                />
                <span className="min-w-0 flex-1 truncate text-xs text-foreground">{itemName}</span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
