// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// recent-reports — the newest documents in the library, each with its
// overall score band, so the dashboard shows the last verdicts the estate
// received without opening the library.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconReportAnalytics } from '@tabler/icons-react';
import {
  ROW_HEIGHT,
  TONE_SOFT,
  WidgetBadge,
  WidgetEmpty,
  WidgetFooterLink,
  WidgetFrame,
  WidgetList,
  WidgetLoading,
  WidgetRow,
  relativeTime,
  useFittedRows,
  type Tone,
} from '@modules/dashboard/components/widget-kit';
import { cn } from '@resources/utils/cn';
import * as api from '../api';
import type { Band } from '../types';
import { useWidgetContext } from './mount';

export const PLUGIN_PATH = '/plugins/addon-reporting';

const BAND_TONE: Record<Band, Tone> = {
  strong: 'success',
  stable: 'success',
  watch: 'warning',
  at_risk: 'danger',
  critical: 'critical',
  not_assessed: 'neutral',
};

function bandTone(band: Band): Tone {
  return BAND_TONE[band] ?? 'neutral';
}

export function RecentReportsWidget() {
  const { t, i18n } = useTranslation('reporting');
  const ago = (iso: string) => relativeTime(iso, t('widgets.common.justNow'), i18n.language);
  const { navigate } = useWidgetContext();
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);
  const reports = useQuery({
    queryKey: ['reporting', 'widget', 'reports'],
    queryFn: () => api.fetchReports(1, 25),
    refetchInterval: 60_000,
  });

  const items = reports.data?.items ?? [];
  const total = reports.data?.total ?? items.length;
  const shown = items.slice(0, capacity);
  const remaining = items.length - shown.length;

  return (
    <WidgetFrame
      title={t('widgets.recentReports.title')}
      badge={
        total > 0 ? <WidgetBadge>{t('widgets.recentReports.inLibrary', { count: total })}</WidgetBadge> : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('widgets.common.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('menu.library')} onClick={() => navigate(`${PLUGIN_PATH}/library`)} />
        </>
      }
    >
      {reports.isLoading ? (
        <WidgetLoading label={t('widgets.common.loading')} />
      ) : reports.isError ? (
        <WidgetEmpty
          icon={<IconReportAnalytics className="size-5" aria-hidden="true" />}
          title={t('widgets.common.unavailable')}
        />
      ) : items.length === 0 ? (
        <WidgetEmpty
          icon={<IconReportAnalytics className="size-5" aria-hidden="true" />}
          title={t('widgets.recentReports.none')}
          hint={t('widgets.recentReports.noneHint')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((r) => {
            const tone = bandTone(r.overall_band);
            const assessed = r.overall_band && r.overall_band !== 'not_assessed';
            return (
              <li key={r.id}>
                <WidgetRow
                  leading={
                    <span
                      className={cn(
                        'grid size-8 shrink-0 place-items-center rounded-md text-xs font-semibold tabular-nums',
                        TONE_SOFT[tone],
                      )}
                    >
                      {assessed ? Math.round(r.overall_score) : '–'}
                    </span>
                  }
                  title={r.title}
                  subtitle={r.headline || r.environment}
                  trailing={<span className="w-12 text-right tabular-nums">{ago(r.created_at)}</span>}
                  onClick={() => navigate(`${PLUGIN_PATH}/library`)}
                />
              </li>
            );
          })}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
