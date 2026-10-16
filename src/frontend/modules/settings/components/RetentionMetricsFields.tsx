// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { SettingsField } from '@resources/components/settings/SettingsGroup';
import type { DEFAULT_METRICS } from './retention-rows';

type Metrics = typeof DEFAULT_METRICS;

type RetentionMetricsFieldsProps = {
  metrics: Metrics;
  onMetricsChange: (next: Metrics) => void;
  recordExec: boolean;
  onRecordExecChange: (value: boolean) => void;
};

/**
 * The two horizons that are not purgeable logs: tiered metric history, and
 * the switch that decides whether Docker exec events are recorded at all.
 *
 * Neither metric horizon accepts 0 as "keep forever" the way the log rows do
 * — an unbounded 30s series would grow without limit — so both `min` at 1.
 * They save with the rest of the group; this component only holds the fields.
 */
export function RetentionMetricsFields({
  metrics,
  onMetricsChange,
  recordExec,
  onRecordExecChange,
}: RetentionMetricsFieldsProps) {
  const { t } = useTranslation('settings');

  return (
    <>
      <div className="grid gap-5 lg:grid-cols-2">
        <SettingsField
          htmlFor="settings-retention-metrics-fields-1"
          label={t('retention.metricsRawHours')}
          description={t('retention.metricsRawHoursDescription')}
        >
          <NumberInput
            id="settings-retention-metrics-fields-1"
            value={metrics.rawHours}
            onChange={(rawHours) => onMetricsChange({ ...metrics, rawHours })}
            min={1}
            max={8760}
            className="w-40"
          />
        </SettingsField>

        <SettingsField
          htmlFor="settings-retention-metrics-fields-2"
          label={t('retention.metricsRollupDays')}
          description={t('retention.metricsRollupDaysDescription')}
        >
          <NumberInput
            id="settings-retention-metrics-fields-2"
            value={metrics.rollupDays}
            onChange={(rollupDays) => onMetricsChange({ ...metrics, rollupDays })}
            min={1}
            max={3650}
            className="w-40"
          />
        </SettingsField>

        <SettingsField
          htmlFor="settings-retention-metrics-fields-3"
          label={t('retention.metricsDisk')}
          description={t('retention.metricsDiskDescription')}
        >
          <Switch
            id="settings-retention-metrics-fields-3"
            checked={metrics.diskEnabled}
            onCheckedChange={(diskEnabled) => onMetricsChange({ ...metrics, diskEnabled })}
            aria-label={t('retention.metricsDisk')}
          />
        </SettingsField>
      </div>

      <SettingsField
        htmlFor="settings-retention-metrics-fields-4"
        label={t('retention.recordExec')}
        description={t('retention.recordExecDescription')}
        className="rounded-lg border border-border bg-background/40 p-4"
      >
        <div className="space-y-3">
          <Switch
            id="settings-retention-metrics-fields-4"
            checked={recordExec}
            onCheckedChange={onRecordExecChange}
            aria-label={t('retention.recordExec')}
          />
          {/* The warning only appears once the switch is on: it is advice
              about a choice being made, not a standing caution. */}
          {recordExec && (
            <Alert
              variant="warning"
              title={t('retention.recordExecWarningTitle')}
              description={t('retention.recordExecWarningBody')}
            />
          )}
        </div>
      </SettingsField>
    </>
  );
}
