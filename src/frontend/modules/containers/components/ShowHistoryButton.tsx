// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconChartLine } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import type { HistoryMetric } from '@resources/components/history-metrics';

type ShowHistoryButtonProps = {
  metric: HistoryMetric;
  /** Translated tile name, used to disambiguate the accessible label. */
  metricName: string;
  active: boolean;
  onSelect: (metric: HistoryMetric) => void;
};

/**
 * Footer action on a stat tile: sends that tile's metric to the history panel.
 *
 * Four of these render side by side with identical visible text, so the
 * accessible name carries the metric — otherwise a screen reader announces
 * "Show history" four times with nothing to tell them apart. The active one
 * renders filled, which is what makes the tile row double as the panel's
 * current-selection indicator.
 */
export function ShowHistoryButton({ metric, metricName, active, onSelect }: ShowHistoryButtonProps) {
  const { t } = useTranslation('containers');

  return (
    <Button
      type="button"
      size="sm"
      variant={active ? 'default' : 'ghost'}
      className="w-full"
      onClick={() => onSelect(metric)}
      aria-pressed={active}
      aria-label={t('stats.showHistoryFor', { metric: metricName })}
    >
      <IconChartLine className="size-3.5" />
      {t('stats.showHistory')}
    </Button>
  );
}
