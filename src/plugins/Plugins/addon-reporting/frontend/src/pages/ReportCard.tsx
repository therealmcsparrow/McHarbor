// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconSparkles } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import type { Level, ReportType } from '../types';
import { typeDescription, typeName } from './catalog-labels';

export function ReportCard({
  type,
  selected,
  onSelect,
}: {
  type: ReportType;
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation('reporting');
  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        'group flex h-full flex-col items-stretch justify-start rounded-xl border bg-card p-4 text-left font-normal whitespace-normal transition-colors',
        selected ? 'border-primary ring-1 ring-primary/30' : 'border-border hover:border-primary/40 hover:bg-muted/30',
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <h3 className="text-sm font-semibold text-foreground">{typeName(t, type)}</h3>
        {/* The catalogue declares `sparkles` on the ad-hoc type and
         * `presentation` on the executive overview (backend/catalog.go);
         * this marker had drifted onto the wrong card, which put an AI
         * icon on a report no model touches and left the one that is
         * written by a model unmarked. It is labelled rather than
         * decorative — it is the only thing on the card that says a model
         * writes this document. */}
        {type.key === 'ad-hoc' ? (
          <>
            <IconSparkles className="size-4 shrink-0 text-primary" aria-hidden="true" />
            <span className="sr-only">{t('studio.aiAuthored')}</span>
          </>
        ) : null}
      </div>
      <p className="mt-1.5 flex-1 text-xs leading-relaxed text-muted-foreground">{typeDescription(t, type)}</p>
      <div className="mt-3 flex flex-wrap gap-1.5">
        {type.levels.map((level: Level) => (
          <span
            key={level}
            className="rounded-full bg-muted px-2 py-0.5 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase"
          >
            {t(`level.${level}`)}
          </span>
        ))}
        {type.point_in_time ? (
          <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-semibold tracking-wider text-primary uppercase">
            {t('studio.pointInTime')}
          </span>
        ) : null}
      </div>
    </Button>
  );
}
