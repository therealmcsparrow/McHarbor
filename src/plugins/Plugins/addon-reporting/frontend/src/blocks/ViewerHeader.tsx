// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconFileTypeDocx, IconFileTypePdf, IconReportAnalytics } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import type { StatusTone } from '@resources/theme/status-palette';
import { formatDateWith } from '../date-format';
import type { Band, Meta, Scorecard } from '../types';

const BAND_TONE: Record<Band, StatusTone> = {
  strong: 'success',
  stable: 'success',
  watch: 'warning',
  at_risk: 'danger',
  critical: 'critical',
  not_assessed: 'neutral',
};

type Props = {
  meta: Meta;
  scorecard?: Scorecard;
  onExport: (format: 'pdf' | 'docx') => void;
  exporting: boolean;
};

export function ViewerHeader({ meta, scorecard, onExport, exporting }: Props) {
  const { t } = useTranslation('reporting');
  const scored = !!scorecard && scorecard.overall >= 0;

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconReportAnalytics className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {meta.title}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            {meta.subtitle ? (
              <span className="min-w-0 truncate text-xs text-muted-foreground">{meta.subtitle}</span>
            ) : null}
            {scored ? (
              <StatusIndicator
                tone={BAND_TONE[scorecard.band]}
                label={`${scorecard.overall} / 100 · ${t(`band.${scorecard.band}`)}`}
                size="sm"
              />
            ) : null}
            <Badge variant="secondary" className="px-1.5 py-0 text-[10px] uppercase">
              {meta.classification}
            </Badge>
            <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {meta.environment}
            </span>
            {meta.subject_label ? (
              <span className="truncate rounded-md bg-primary/10 px-1.5 py-0.5 text-[10px] font-semibold text-primary">
                {t('viewer.scope', { subject: meta.subject_label })}
              </span>
            ) : null}
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {formatPeriod(meta.period_start, meta.period_end)}
            </span>
            <span className="truncate text-[10px] text-muted-foreground">
              {t('viewer.generatedBy', { user: meta.generated_by })}
            </span>
          </div>
        </div>
      </PageHeadIdentity>

      <PageHeadActions>
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          <HeaderActionButton
            tooltip={t('action.exportPdf')}
            onClick={() => onExport('pdf')}
            disabled={exporting}
            icon={<IconFileTypePdf className="size-3.5" />}
          />
          <HeaderActionButton
            tooltip={t('action.exportDocx')}
            onClick={() => onExport('docx')}
            disabled={exporting}
            icon={<IconFileTypeDocx className="size-3.5" />}
          />
        </div>
      </PageHeadActions>
    </>
  );
}

function formatPeriod(from: string, to: string): string {
  const a = new Date(from);
  const b = new Date(to);
  if (Number.isNaN(a.getTime()) || Number.isNaN(b.getTime())) return '—';
  if (Math.abs(b.getTime() - a.getTime()) < 60_000) {
    return formatDateWith(b, { dateStyle: 'medium' });
  }
  return `${formatDateWith(a, { day: 'numeric', month: 'short' })} – ${formatDateWith(b, { dateStyle: 'medium' })}`;
}
