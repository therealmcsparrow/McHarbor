// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconChevronRight, IconFile, IconFolder, IconFolderOpen } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { ContainerPathChildren } from './ContainerPathTree';

// Picking a folder selects everything beneath it, so a child whose
// ancestor is already selected is covered and shows as such.
function coveredBy(path: string, selected: string[]) {
  return selected.some((entry) => entry === path || path.startsWith(`${entry.replace(/\/$/, '')}/`));
}

// Tailwind needs literal class names, so the indent steps are enumerated
// rather than interpolated. Ten levels is deeper than any realistic pick.
function depthClass(depth: number) {
  const steps = ['w-0', 'w-3', 'w-6', 'w-9', 'w-12', 'w-16', 'w-20', 'w-24', 'w-28', 'w-32'];
  return `${steps[Math.min(depth, steps.length - 1)]} shrink-0`;
}

type ContainerPathNodeProps = {
  containerId: string;
  rootPath: string;
  path: string;
  name: string;
  isDir: boolean;
  depth: number;
  selected: string[];
  onChange: (paths: string[]) => void;
};

// One row of the ContainerPathTree: a file, or a folder that lazily loads
// its children when opened.
export function ContainerPathNode({
  containerId,
  rootPath,
  path,
  name: itemName,
  isDir,
  depth,
  selected,
  onChange,
}: ContainerPathNodeProps) {
  const { t } = useTranslation('containers');
  const [isOpen, setOpen] = useState(false);
  const covered = coveredBy(path, selected);
  const explicit = selected.includes(path);

  function toggle(next: boolean) {
    if (next) {
      // Selecting a folder makes any already-selected descendant
      // redundant, so they are dropped rather than left to accumulate as
      // noise in the stored plan.
      const prefix = `${path.replace(/\/$/, '')}/`;
      onChange([...selected.filter((entry) => entry !== path && !entry.startsWith(prefix)), path]);
      return;
    }
    onChange(selected.filter((entry) => entry !== path));
  }

  return (
    <div>
      <div className="flex items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-muted/40">
        <span className={depthClass(depth)} aria-hidden="true" />
        {isDir ? (
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
        {isDir ? (
          isOpen ? (
            <IconFolderOpen className="size-4 shrink-0 text-warning" aria-hidden="true" />
          ) : (
            <IconFolder className="size-4 shrink-0 text-warning" aria-hidden="true" />
          )
        ) : (
          <IconFile className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        )}
        <Checkbox
          checked={covered}
          disabled={covered && !explicit}
          onCheckedChange={(next) => toggle(next === true)}
          aria-label={path}
          className="shrink-0"
        />
        <span className="min-w-0 flex-1 truncate text-sm text-foreground">{itemName}</span>
      </div>
      {isDir && isOpen && (
        <ContainerPathChildren
          containerId={containerId}
          rootPath={rootPath}
          path={path}
          depth={depth + 1}
          selected={selected}
          onChange={onChange}
        />
      )}
    </div>
  );
}
