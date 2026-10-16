// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFileAnalytics, IconSearch, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Input } from '@resources/components/ui/Input';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { useCatalog, useCreateSchedule, useStartGenerateJob } from '../hooks/useReporting';
import type { ReportType } from '../types';
import { categoryDescription, categoryLabel, typeDescription, typeName } from './catalog-labels';
import { ConfigureDialog } from './ConfigureDialog';
import { GenerateProgressDialog } from './GenerateProgressDialog';
import { ReportCard } from './ReportCard';

// Report Studio: choose what to report on, then how.
//
// The catalog leads because the first decision is which question is being
// asked; period and scope only make sense once that is settled. Types are
// grouped by audience so a director and an engineer each find their own
// shelf first.
//
// The order is a reading order, not an alphabet: the board report, then how
// the service performed, then the per-workload depth behind it, then the
// evidence an auditor asks for, then the machinery, the estate, and finally
// what it all costs. A category with no types is dropped by the grouping
// below, so an installation missing a module never shows an empty shelf.

type Props = { onOpen: (id: string) => void };

const CATEGORY_ORDER = [
  'custom',
  'board',
  'service',
  'workload',
  'assurance',
  'operations',
  'platform',
  // Swarm is a *contributed* shelf — the Swarm add-on registers those three
  // types through the reporting adapter family, and this plugin no longer
  // declares them. It is named here anyway, and it is the one entry in this
  // list that is a preference rather than an inventory: a contributed shelf
  // named here keeps its position, and one that is not is appended after the
  // built-ins. Swarm answers the same kind of question the platform shelf
  // does, one level up — the cluster rather than the host — so it reads
  // better here than at the end. An installation without the add-on has no
  // types in it and the grouping below drops the shelf entirely.
  'swarm',
  'finance',
];

export function StudioPage({ onOpen }: Props) {
  const { t } = useTranslation('reporting');
  const catalog = useCatalog();
  const generate = useStartGenerateJob();
  const createSchedule = useCreateSchedule();
  const [selected, setSelected] = useState<ReportType | null>(null);
  const [jobId, setJobId] = useState<string | null>(null);
  const [query, setQuery] = useState('');

  // Generation is a job, so the configure dialog closes the moment the
  // job is accepted and the progress dialog takes over. Holding the
  // configure dialog open behind a spinner would keep the operator
  // looking at controls that no longer do anything.
  const openGenerated = useCallback(
    (reportId: string) => {
      setJobId(null);
      onOpen(reportId);
    },
    [onOpen],
  );

  const grouped = useMemo(() => {
    const all = catalog.data?.types ?? [];
    // Matched on what the card actually shows — the translated name, the
    // description and the shelf it sits on — not on the type key, which is a
    // machine value no operator has seen.
    const needle = query.trim().toLowerCase();
    const types = needle
      ? all.filter((x) =>
          [typeName(t, x), typeDescription(t, x), categoryLabel(t, x.category, x)].some((field) =>
            field?.toLowerCase().includes(needle),
          ),
        )
      : all;
    // The reading order above is this plugin's own shelves. A plugin
    // that contributed a shelf of its own is appended after them, in
    // catalog order: it has no place in a sequence written around the
    // built-in types, and dropping it — which a fixed list silently did
    // — would hide a report the operator installed a plugin to get.
    const extra = all
      .map((x) => x.category)
      .filter((c) => c && !CATEGORY_ORDER.includes(c))
      .filter((c, i, seen) => seen.indexOf(c) === i);
    return [...CATEGORY_ORDER, ...extra]
      .map((category) => ({
        category,
        types: types.filter((x) => x.category === category),
      }))
      .filter((g) => g.types.length > 0);
  }, [catalog.data, query, t]);

  const total = catalog.data?.types.length ?? 0;
  const shown = grouped.reduce((sum, group) => sum + group.types.length, 0);

  if (catalog.isLoading) {
    return (
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {[0, 1, 2, 3, 4, 5].map((slot) => (
          <CardSkeleton key={slot} lines={3} />
        ))}
      </div>
    );
  }

  if (catalog.isError || total === 0) {
    return (
      <EmptyState
        icon={<IconFileAnalytics className="size-8" />}
        title={t('studio.empty.title')}
        description={t('studio.empty.description')}
      />
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="relative w-full max-w-sm">
          <IconSearch
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            value={query}
            onChange={(evt) => setQuery(evt.target.value)}
            placeholder={t('studio.searchPlaceholder')}
            className="pr-8 pl-8"
          />
          {query !== '' && (
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.clear', { ns: 'common' })}
              className="absolute top-1/2 right-1 -translate-y-1/2"
              onClick={() => setQuery('')}
            >
              <IconX className="size-4" aria-hidden="true" />
            </Button>
          )}
        </div>
        {query !== '' && (
          <span className="text-sm text-muted-foreground">{t('studio.searchCount', { shown, total })}</span>
        )}
      </div>

      {grouped.length === 0 ? (
        <EmptyState
          icon={<IconSearch className="size-8" />}
          title={t('studio.searchEmptyTitle')}
          description={t('studio.searchEmptyDescription')}
        />
      ) : null}

      {grouped.map((group) => (
        <section key={group.category}>
          <h2 className="mb-1 text-sm font-semibold tracking-[0.12em] text-muted-foreground uppercase">
            {categoryLabel(t, group.category, group.types[0])}
          </h2>
          <p className="mb-3 text-sm text-muted-foreground">{categoryDescription(t, group.category, group.types[0])}</p>
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {group.types.map((type) => (
              <ReportCard
                key={type.key}
                type={type}
                selected={selected?.key === type.key}
                onSelect={() => setSelected(type)}
              />
            ))}
          </div>
        </section>
      ))}

      <ConfigureDialog
        type={selected}
        pending={generate.isPending || createSchedule.isPending}
        error={generate.isError || createSchedule.isError}
        onClose={() => setSelected(null)}
        onGenerate={(request) => {
          generate.mutate(request, {
            onSuccess: (job) => {
              setSelected(null);
              setJobId(job.id);
            },
          });
        }}
        // Creating a schedule produces nothing to open: the first run
        // happens on the cron, not now. The dialog simply closes, and the
        // Report Schedules entry is where the result appears.
        onSchedule={(request) => {
          createSchedule.mutate(request, { onSuccess: () => setSelected(null) });
        }}
      />

      <GenerateProgressDialog jobId={jobId} onDone={openGenerated} onClose={() => setJobId(null)} />
    </div>
  );
}
