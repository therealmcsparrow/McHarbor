// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconChevronRight, IconSelector } from '@tabler/icons-react';
import { Popover, PopoverContent, PopoverTrigger } from '@resources/components/ui/Popover';
import { Button } from '@resources/components/ui/Button';
import { useHeaderSlot } from '@resources/hooks/useHeaderSlot';
import { cn } from '@resources/utils/cn';
import { PluginLink } from '../nav-context';

export type PluginCrumb = {
  label: ReactNode;
  /** Plugin-relative, e.g. `nodes`. Omit for a crumb that is not a link. */
  to?: string;
  icon?: ReactNode;
};

export type PluginSibling = {
  id: string;
  label: string;
  /** Plugin-relative, e.g. `nodes/abc123`. */
  to: string;
  /** Optional state adornment — a dot, a badge. */
  adornment?: ReactNode;
};

type Props = {
  /** Ancestors, root first. The last entry is the leaf. */
  crumbs: PluginCrumb[];
  /** Peers of the leaf — turns the leaf into a switcher. */
  siblings?: PluginSibling[];
  /** id of the current leaf within `siblings`. */
  currentId?: string;
  className?: string;
};

export function PluginBreadcrumb({ crumbs, siblings, currentId, className }: Props) {
  const { t } = useTranslation('common');
  const [isOpen, setOpen] = useState(false);
  const slot = useHeaderSlot();

  if (crumbs.length === 0) return null;
  if (!slot) return null;
  const leaf = crumbs[crumbs.length - 1]!;
  const ancestors = crumbs.slice(0, -1);
  // One sibling is the page you are already on, so a switcher would offer a
  // single choice that changes nothing.
  const hasSwitcher = Boolean(siblings && siblings.length > 1);

  return createPortal(
    <nav aria-label={t('breadcrumb.label')} className={cn('flex min-w-0 items-center gap-1 text-sm', className)}>
      {ancestors.map((crumb, depth) => (
        <span key={depth} className="flex shrink-0 items-center gap-1">
          {crumb.to ? (
            <PluginLink
              to={crumb.to}
              className="inline-flex items-center gap-1 text-muted-foreground transition-colors hover:text-foreground"
            >
              {crumb.icon}
              {crumb.label}
            </PluginLink>
          ) : (
            <span className="inline-flex items-center gap-1 text-muted-foreground">
              {crumb.icon}
              {crumb.label}
            </span>
          )}
          <IconChevronRight className="size-3.5 shrink-0 text-muted-foreground/50" aria-hidden="true" />
        </span>
      ))}

      {hasSwitcher ? (
        <Popover open={isOpen} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="min-w-0 gap-1 px-1.5 font-semibold text-foreground"
              aria-label={t('breadcrumb.switchSibling')}
            >
              <span className="truncate">{leaf.label}</span>
              <IconSelector className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="start" className="max-h-80 w-64 overflow-y-auto p-1">
            <ul className="flex flex-col">
              {siblings!.map((sibling) => (
                <li key={sibling.id}>
                  <PluginLink
                    to={sibling.to}
                    onClick={() => setOpen(false)}
                    aria-current={sibling.id === currentId ? 'page' : undefined}
                    className={cn(
                      'flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors',
                      'hover:bg-accent focus-visible:bg-accent',
                      sibling.id === currentId ? 'text-foreground' : 'text-muted-foreground',
                    )}
                  >
                    <IconCheck
                      className={cn('size-3.5 shrink-0', sibling.id === currentId ? 'opacity-100' : 'opacity-0')}
                      aria-hidden="true"
                    />
                    {sibling.adornment}
                    <span className="truncate">{sibling.label}</span>
                  </PluginLink>
                </li>
              ))}
            </ul>
          </PopoverContent>
        </Popover>
      ) : (
        <span className="min-w-0 truncate font-semibold text-foreground">{leaf.label}</span>
      )}
    </nav>,
    slot,
  );
}
