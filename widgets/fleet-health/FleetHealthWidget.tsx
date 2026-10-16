// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// fleet-health — the widget that answers "is anything wrong?"
//
// The 33-type catalogue had no widget that returned a verdict. Every one
// of them returned a reading: CPU is at 34%, there are 12 containers, disk
// is 41% full. All true, none of them an answer. Opening McHarbor told you
// the state of the world and left the judgement to you, every time, even
// though the judgement is the same every time.
//
// So it reads top-down as a paragraph, not a panel. The verdict is one
// sentence in two clauses — what is up, then what wants you — the facts
// under it, the split beneath that, and only then the two things to click.
// Every number is click-through: a segment lands on the container list
// already filtered to that state, a card lands on the thing itself.

import { cssVars } from '@resources/utils/cssVars';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconArrowRight, IconFileText, IconRefresh } from '@tabler/icons-react';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { useCheckContainerUpdates } from '@modules/containers/hooks/useContainerUpdates';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import { MAX_CARDS, type AttentionItem } from './fleetHealthModel';
import { useFleetHealth } from './useFleetHealth';

/** Tile tint per card tone. Kept as whole classes so Tailwind can see them. */
const CARD_TONE: Record<AttentionItem['tone'], string> = {
  danger: 'bg-danger/15 text-danger',
  warning: 'bg-warning/15 text-warning',
  info: 'bg-info/15 text-info',
  neutral: 'bg-neutral/15 text-neutral',
  success: 'bg-success/15 text-success',
};

export default function FleetHealthWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const checkUpdates = useCheckContainerUpdates();
  const { isLoading, isError, total, segments, down, facts, items } = useFleetHealth();

  if (isError) {
    return (
      <div className="flex h-full items-center justify-center px-4 text-sm text-muted-foreground">
        {t('fleetHealthWidget.unavailable')}
      </div>
    );
  }

  // Clause one is about what is *down* — dead or stuck restarting. A
  // container someone stopped on purpose, or one failing its health check
  // while still serving traffic, is not down; it is clause two's problem.
  const state = isLoading
    ? t('fleetHealthWidget.checking')
    : total === 0
      ? t('fleetHealthWidget.empty')
      : down > 0
        ? t('fleetHealthWidget.verdict.down', { count: down })
        : t('fleetHealthWidget.verdict.allUp');

  const wants =
    isLoading || total === 0
      ? null
      : items.length === 0 || items[0]?.id === 'all-clear'
        ? t('fleetHealthWidget.wants.none')
        : t('fleetHealthWidget.wants.some', { count: items.length });

  const nonEmpty = segments.filter((segment) => segment.count > 0);

  return (
    <div className="flex h-full flex-col gap-3 overflow-hidden px-4 pt-3 pb-3">
      <div className="flex shrink-0 flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="min-w-0">
          <p className="text-lg leading-snug font-semibold text-foreground">
            {state} {wants && <span className="text-primary">{wants}</span>}
          </p>
          <p className="mt-1 truncate text-xs text-muted-foreground">{facts.join(' · ')}</p>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={checkUpdates.isPending}
            onClick={() => checkUpdates.mutate(undefined)}
          >
            {checkUpdates.isPending ? <Spinner size="sm" /> : <IconRefresh className="size-4" aria-hidden="true" />}
            {t('fleetHealthWidget.actions.checkUpdates')}
          </Button>
          <Button variant="outline" size="sm" onClick={() => navigate('/logs')}>
            <IconFileText className="size-4" aria-hidden="true" />
            {t('fleetHealthWidget.actions.liveLogs')}
          </Button>
        </div>
      </div>

      {total > 0 && (
        <div className="shrink-0">
          {/* One bar, proportional, and thick enough to carry its own
              numbers — reading it takes no counting and no legend. */}
          <fieldset
            className="m-0 flex h-9 w-full min-w-0 gap-0.5 border-0 p-0"
            aria-label={t('fleetHealthWidget.barLabel', { count: total })}
          >
            {nonEmpty.map((segment) => (
              <PlainButton
                key={segment.key}
                type="button"
                onClick={() => navigate(`/containers?state=${segment.facet}`)}
                aria-label={`${segment.count} ${t(`fleetHealthWidget.state.${segment.key}`)}`}
                className={cn(
                  'flex min-w-9 [flex-grow:var(--grow)] basis-0 items-center justify-center rounded-sm text-sm font-semibold tabular-nums transition-opacity first:rounded-l-lg last:rounded-r-lg hover:opacity-85',
                  segment.barClass,
                  segment.barTextClass,
                )}
                // Flex growth is the data; it travels as a CSS variable the
                // class reads.
                style={cssVars({ grow: segment.count })}
              >
                {segment.count}
              </PlainButton>
            ))}
          </fieldset>

          <div className="mt-2 flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
              {nonEmpty.map((segment) => (
                <PlainButton
                  key={segment.key}
                  type="button"
                  onClick={() => navigate(`/containers?state=${segment.facet}`)}
                  className="group inline-flex items-center gap-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
                >
                  <span aria-hidden="true" className={cn('size-2 shrink-0 rounded-full', segment.dotClass)} />
                  {t(`fleetHealthWidget.state.${segment.key}`)}
                  <span className="font-semibold text-foreground tabular-nums">{segment.count}</span>
                </PlainButton>
              ))}
            </div>
            <span className="text-xs text-muted-foreground">{t('fleetHealthWidget.subtitle', { count: total })}</span>
          </div>
        </div>
      )}

      {items.length > 0 && (
        <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 overflow-y-auto sm:grid-cols-2">
          {items.slice(0, MAX_CARDS).map((item) => (
            <PlainButton
              key={item.id}
              type="button"
              onClick={() => navigate(item.to)}
              className="flex items-center gap-3 rounded-xl border border-hairline bg-glass px-3 py-2.5 text-left transition-colors hover:border-hairline-2 hover:bg-muted/40"
            >
              <span className={cn('grid size-9 shrink-0 place-items-center rounded-lg', CARD_TONE[item.tone])}>
                <item.icon className="size-5" aria-hidden="true" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-semibold text-foreground">{item.title}</span>
                <span className="mt-0.5 block truncate text-xs text-muted-foreground">{item.detail}</span>
              </span>
              <IconArrowRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            </PlainButton>
          ))}
        </div>
      )}
    </div>
  );
}
