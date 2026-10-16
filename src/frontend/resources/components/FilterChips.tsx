// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// FilterChips — state facets with live counts.
//
// "Show me what's unhealthy" used to require a regex, or knowing the names
// already. The search toolbar offered contains / exact / regex over text
// and saved filters, but nothing that knew a container could be *unhealthy*
// — which is the question people actually arrive with.
//
// These chips compose with the existing query rather than replacing it: the
// text search still applies, regex still applies, saved filters still apply.
// The chip narrows what the query returns. Chip state joins the URL, so a
// filtered view is a shareable link.

import { PlainButton } from '@resources/components/ui/PlainButton';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import type { StatusTone } from '@resources/theme/status-palette';

export type FilterFacet<T extends string> = {
  id: T;
  label: string;
  /** Number of items this facet would show. Rendered beside the label. */
  count: number;
  /** Optional tone. `undefined` reads as neutral — most facets should. */
  tone?: StatusTone;
};

type FilterChipsProps<T extends string> = {
  facets: readonly FilterFacet<T>[];
  value: T;
  onChange: (next: T) => void;
  /** Accessible name for the group. Already translated. */
  label?: string;
  className?: string;
};

const TONE_ACTIVE: Record<StatusTone, string> = {
  success: 'border-success/50 bg-success/15 text-success',
  warning: 'border-warning/50 bg-warning/15 text-warning',
  danger: 'border-danger/50 bg-danger/15 text-danger',
  critical: 'border-critical/50 bg-critical/15 text-critical',
  info: 'border-info/50 bg-info/15 text-info',
  neutral: 'border-neutral/50 bg-neutral/15 text-neutral',
};

export function FilterChips<T extends string>({ facets, value, onChange, label, className }: FilterChipsProps<T>) {
  const { t } = useTranslation('common');

  return (
    <fieldset
      aria-label={label ?? t('filters.facets')}
      className={cn('m-0 flex min-w-0 flex-wrap items-center gap-1.5 border-0 p-0', className)}
    >
      {facets.map((facet) => {
        const active = facet.id === value;
        const empty = facet.count === 0;
        return (
          <PlainButton
            key={facet.id}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(facet.id)}
            className={cn(
              'mc-control inline-flex items-center gap-1.5 border px-2.5',
              'text-xs font-medium transition-colors',
              'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
              active
                ? facet.tone
                  ? TONE_ACTIVE[facet.tone]
                  : 'border-primary/50 bg-primary/15 text-primary'
                : 'border-border bg-transparent text-muted-foreground hover:border-border hover:bg-muted hover:text-foreground',
              // An empty facet is still clickable — the zero is information,
              // and hiding it would make the row jump as counts change.
              empty && !active && 'opacity-55',
            )}
          >
            {facet.label}
            <span className={cn('tabular-nums', active ? 'opacity-80' : 'text-muted-foreground/70')}>
              {facet.count}
            </span>
          </PlainButton>
        );
      })}
    </fieldset>
  );
}
