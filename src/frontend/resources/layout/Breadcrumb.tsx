// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Breadcrumb — where you are, and how to leave sideways.
//
// A container detail page used to offer a bare left-arrow: one target, no
// statement of where you were. That is fine when you arrived from the list
// and wrong when you arrived from a search result, a deep link, or the
// dashboard, which is most of the time.
//
// The leaf carries a sibling switcher, because "back to the list, find the
// next one, click it" is three steps to do the thing people are actually
// doing — comparing two containers.

import { useState, type ReactNode } from 'react';
import { Link, useInRouterContext } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconChevronRight, IconSelector } from '@tabler/icons-react';
import { Popover, PopoverContent, PopoverTrigger } from '@resources/components/ui/Popover';
import { Button } from '@resources/components/ui/Button';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { cn } from '@resources/utils/cn';

export type BreadcrumbCrumb = {
  label: ReactNode;
  to?: string;
  icon?: ReactNode;
  onClick?: () => void;
};

export type BreadcrumbSibling = {
  id: string;
  label: string;
  to: string;
  /** Optional state adornment — a dot, a badge. */
  adornment?: ReactNode;
};

type BreadcrumbProps = {
  /** Ancestors, root first. The last entry is the leaf. */
  crumbs: BreadcrumbCrumb[];
  /**
   * Peers of the leaf. When present, the leaf becomes a switcher so you
   * can move between them without going back up.
   */
  siblings?: BreadcrumbSibling[];
  /** id of the current leaf within `siblings`. */
  currentId?: string;
  /** Renders the leaf inside an `<h1>` — every page needs exactly one. */
  headingLeaf?: boolean;
  className?: string;
};

export function Breadcrumb({ crumbs, siblings, currentId, headingLeaf, className }: BreadcrumbProps) {
  const { t } = useTranslation('common');
  const [isOpen, setOpen] = useState(false);
  const inRouter = useInRouterContext();

  if (crumbs.length === 0) return null;
  const leaf = crumbs[crumbs.length - 1]!;
  const ancestors = crumbs.slice(0, -1);
  const hasSwitcher = Boolean(siblings && siblings.length > 1);

  return (
    <nav aria-label={t('breadcrumb.label')} className={cn('flex min-w-0 items-center gap-1 text-sm', className)}>
      {ancestors.map((crumb, index) => (
        <span key={crumb.to ?? `crumb-${index}`} className="flex shrink-0 items-center gap-1">
          {crumb.onClick ? (
            <PlainButton
              onClick={crumb.onClick}
              className="inline-flex items-center gap-1 text-muted-foreground transition-colors hover:text-foreground"
            >
              {crumb.icon}
              {crumb.label}
            </PlainButton>
          ) : crumb.to && inRouter ? (
            <Link
              to={crumb.to}
              className="inline-flex items-center gap-1 text-muted-foreground transition-colors hover:text-foreground"
            >
              {crumb.icon}
              {crumb.label}
            </Link>
          ) : crumb.to ? (
            <a
              href={crumb.to}
              className="inline-flex items-center gap-1 text-muted-foreground transition-colors hover:text-foreground"
            >
              {crumb.icon}
              {crumb.label}
            </a>
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
                  <Link
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
                  </Link>
                </li>
              ))}
            </ul>
          </PopoverContent>
        </Popover>
      ) : headingLeaf ? (
        <h1 className="min-w-0 truncate font-display text-sm leading-tight font-semibold tracking-tight text-foreground">
          {leaf.label}
        </h1>
      ) : (
        <span className="min-w-0 truncate font-semibold text-foreground">{leaf.label}</span>
      )}
    </nav>
  );
}
