// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import type { Finding, KPI, Pair, Scorecard, Status } from '../types';
import {
  bandStatus,
  severityStatus,
  statusAccent,
  statusBarInk,
  statusPill,
  statusSurface,
  statusText,
} from './tokens';

// The non-chart block renderers.
//
// Each mirrors what the PDF and Word exporters draw for the same block,
// so a reader who has the document on screen and the exported file open
// beside them sees the same thing arranged the same way.

const SCORE_MAX = 100;

export function KpiRow({ items }: { items: KPI[] }) {
  if (items.length === 0) return null;
  return (
    <div className="mb-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {items.map((k, i) => {
        const statusValue: Status = k.status ?? '';
        return (
          <div
            key={`${k.label}-${i}`}
            className={cn('relative overflow-hidden rounded-xl border p-4', statusSurface[statusValue])}
          >
            {statusValue ? (
              <span
                aria-hidden="true"
                className={cn('absolute inset-y-3 left-0 w-[3px] rounded-full', statusAccent[statusValue])}
              />
            ) : null}
            <p className="text-[10px] font-semibold tracking-[0.12em] text-muted-foreground uppercase">{k.label}</p>
            <p
              className={cn(
                'mt-1 text-2xl font-bold tabular-nums',
                statusValue ? statusText[statusValue] : 'text-foreground',
              )}
            >
              {k.value}
              {k.unit ? <span className="ml-0.5 text-base font-semibold">{k.unit}</span> : null}
            </p>
            {k.target || k.hint ? (
              <p className="mt-1 text-[11px] text-muted-foreground">
                {k.target && k.hint ? `vs ${k.target} · ${k.hint}` : k.target ? `vs ${k.target}` : k.hint}
              </p>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

export function Callout({ tone, title, text }: { tone: Status; title?: string; text: string }) {
  const statusValue: Status = tone || 'info';
  return (
    <div className={cn('mb-5 flex gap-3 rounded-lg border p-4', statusSurface[statusValue])}>
      <span aria-hidden="true" className={cn('w-[3px] shrink-0 rounded-full', statusAccent[statusValue])} />
      <div className="min-w-0">
        {title ? <p className={cn('text-sm font-semibold', statusText[statusValue])}>{title}</p> : null}
        <p className="mt-0.5 text-sm leading-relaxed text-foreground">{text}</p>
      </div>
    </div>
  );
}

export function KeyValues({ title, pairs }: { title?: string; pairs: Pair[] }) {
  if (pairs.length === 0) return null;
  return (
    <div className="mb-6">
      {title ? <h5 className="mb-2 text-sm font-semibold text-primary">{title}</h5> : null}
      <dl className="divide-y divide-border overflow-hidden rounded-lg border border-border">
        {pairs.map((p, i) => (
          <div key={`${p.k}-${i}`} className={cn('grid gap-1 px-3 py-2 sm:grid-cols-3', i % 2 === 1 && 'bg-muted/25')}>
            <dt className="text-xs font-semibold text-muted-foreground">{p.k}</dt>
            <dd className={cn('text-sm text-foreground sm:col-span-2', p.mono && 'font-mono text-xs')}>{p.v || '—'}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

export function Findings({ items }: { items: Finding[] }) {
  const { t } = useTranslation('reporting');
  if (items.length === 0) return null;
  return (
    <div className="mb-5 space-y-3">
      {items.map((f) => {
        const statusValue = severityStatus(f.severity);
        return (
          <div
            key={`${f.severity}:${f.title}`}
            className={cn('flex gap-3 rounded-lg border p-4', statusSurface[statusValue])}
          >
            <span aria-hidden="true" className={cn('w-[3px] shrink-0 rounded-full', statusAccent[statusValue])} />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <p className="text-sm font-semibold text-foreground">{f.title}</p>
                <span
                  className={cn(
                    'rounded-full px-2 py-0.5 text-[10px] font-bold tracking-wider uppercase',
                    statusPill[statusValue],
                  )}
                >
                  {f.severity}
                </span>
              </div>
              {f.detail ? <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{f.detail}</p> : null}
              {f.recommendation ? (
                <p className="mt-2 text-xs leading-relaxed text-foreground">
                  <span className="font-semibold">{t('findings.recommended')} </span>
                  {f.recommendation}
                </p>
              ) : null}
              {f.owner ? (
                <p className="mt-1.5 text-[11px] text-muted-foreground">{t('findings.owner', { owner: f.owner })}</p>
              ) : null}
            </div>
          </div>
        );
      })}
    </div>
  );
}

export function ScorecardBlock({ scorecard }: { scorecard: Scorecard }) {
  const { t } = useTranslation('reporting');
  if (scorecard.overall < 0) return null;
  const statusValue = bandStatus(scorecard.band);

  return (
    <div className={cn('mb-6 rounded-xl border p-5', statusSurface[statusValue])}>
      <div className="flex flex-col gap-5 sm:flex-row sm:items-center">
        <div className="shrink-0">
          <p className="text-[10px] font-semibold tracking-[0.12em] text-muted-foreground uppercase">
            {t('scorecard.overall')}
          </p>
          <p className="mt-0.5 flex items-baseline gap-1.5">
            <span className="text-4xl font-bold text-primary tabular-nums">{scorecard.overall}</span>
            <span className="text-sm text-muted-foreground">{`/ ${SCORE_MAX}`}</span>
          </p>
          <span
            className={cn(
              'mt-1.5 inline-block rounded-full px-2.5 py-0.5 text-xs font-semibold',
              statusPill[statusValue],
            )}
          >
            {t(`band.${scorecard.band}`)}
          </span>
        </div>

        <dl className="min-w-0 flex-1 space-y-1.5">
          {scorecard.domains
            .filter((d) => d.score >= 0)
            .map((d) => {
              const ds = bandStatus(d.band);
              return (
                <div key={d.key} className="flex items-center gap-3">
                  <dt className="w-24 shrink-0 truncate text-xs text-muted-foreground">{d.label}</dt>
                  <dd className="flex min-w-0 flex-1 items-center gap-2">
                    {/* Drawn as SVG rather than a width-styled div: the
                        width is data, and this keeps it out of an inline
                        style while giving the fill a real status colour.
                        Colour arrives through `currentColor` so the bar
                        needs no `fill-*` utility — see statusBarInk. */}
                    <svg
                      viewBox="0 0 100 6"
                      preserveAspectRatio="none"
                      aria-hidden="true"
                      className="h-1.5 min-w-0 flex-1"
                    >
                      <rect
                        width={100}
                        height={6}
                        rx={3}
                        className="text-muted-foreground"
                        fill="currentColor"
                        fillOpacity={0.22}
                      />
                      <rect
                        width={Math.max(d.score, 2)}
                        height={6}
                        rx={3}
                        className={cn(statusBarInk[ds])}
                        fill="currentColor"
                      />
                    </svg>
                    <span className="w-7 shrink-0 text-right text-xs font-semibold text-foreground tabular-nums">
                      {d.score}
                    </span>
                  </dd>
                </div>
              );
            })}
        </dl>
      </div>
    </div>
  );
}
