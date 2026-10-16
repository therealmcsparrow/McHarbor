// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import type { AutoUpdateDay } from '../hooks/useEnvironmentDetailState';
import { SettingRow } from './ActivitySettingRow';
import { AutoUpdateScheduleFields } from './AutoUpdateScheduleFields';

type EnvironmentRetentionTabProps = {
  logRetentionDays: string;
  containerPruneEnabled: boolean;
  containerPruneStoppedDays: string;
  autoUpdateEnabled: boolean;
  autoUpdateWindowStart: string;
  autoUpdateWindowEnd: string;
  autoUpdateDaysSelected: Set<AutoUpdateDay>;
  metricRetentionHours: string;
  isSaving: boolean;
  onLogRetentionChange: (value: string) => void;
  onContainerPruneEnabledChange: (checked: boolean) => void;
  onContainerPruneStoppedDaysChange: (value: string) => void;
  onAutoUpdateEnabledChange: (checked: boolean) => void;
  onAutoUpdateWindowStartChange: (value: string) => void;
  onAutoUpdateWindowEndChange: (value: string) => void;
  onAutoUpdateDayToggle: (day: string) => void;
  onMetricRetentionChange: (value: string) => void;
};

export function EnvironmentRetentionTab({
  logRetentionDays,
  containerPruneEnabled,
  containerPruneStoppedDays,
  autoUpdateEnabled,
  autoUpdateWindowStart,
  autoUpdateWindowEnd,
  autoUpdateDaysSelected,
  metricRetentionHours,
  isSaving,
  onLogRetentionChange,
  onContainerPruneEnabledChange,
  onContainerPruneStoppedDaysChange,
  onAutoUpdateEnabledChange,
  onAutoUpdateWindowStartChange,
  onAutoUpdateWindowEndChange,
  onAutoUpdateDayToggle,
  onMetricRetentionChange,
}: EnvironmentRetentionTabProps) {
  const { t } = useTranslation('environments');

  const dayLabels: Record<string, string> = t('detail.retention.daysShort', { returnObjects: true }) as Record<
    string,
    string
  >;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.retention.title')}</CardTitle>
        <CardDescription>{t('detail.retention.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <SettingRow
          title={t('detail.retention.logRetentionTitle')}
          description={t('detail.retention.logRetentionDescription')}
          checked={logRetentionDays !== '0'}
          disabled={isSaving}
          onCheckedChange={(checked) => onLogRetentionChange(checked ? '7' : '0')}
          statusOn={t('detail.retention.statusActive')}
          statusOff={t('detail.retention.statusForever')}
        >
          <div className="max-w-xs space-y-2">
            <label className="block text-xs font-semibold tracking-[0.14em] text-muted-foreground uppercase">
              {t('detail.retention.logRetentionLabel')}
            </label>
            <div className="flex items-center gap-3">
              <NumberInput
                nullable
                min={0}
                max={3650}
                aria-label={t('detail.retention.logRetentionLabel')}
                value={toNumberOrNull(logRetentionDays)}
                disabled={logRetentionDays === '0' || isSaving}
                onChange={(next) => onLogRetentionChange(toNumberText(next))}
                suffix={t('detail.retention.daysUnit')}
              />
            </div>
            <p className="text-xs text-muted-foreground">{t('detail.retention.logRetentionHint')}</p>
          </div>
        </SettingRow>

        <SettingRow
          title={t('detail.retention.containerPruneTitle')}
          description={t('detail.retention.containerPruneDescription')}
          checked={containerPruneEnabled}
          disabled={isSaving}
          onCheckedChange={onContainerPruneEnabledChange}
          statusOn={t('detail.retention.statusOn')}
          statusOff={t('detail.retention.statusOff')}
        >
          <div className="max-w-xs space-y-2">
            <label className="block text-xs font-semibold tracking-[0.14em] text-muted-foreground uppercase">
              {t('detail.retention.containerPruneStoppedLabel')}
            </label>
            <div className="flex items-center gap-3">
              <NumberInput
                nullable
                min={1}
                max={3650}
                aria-label={t('detail.retention.containerPruneStoppedLabel')}
                value={toNumberOrNull(containerPruneStoppedDays)}
                disabled={!containerPruneEnabled || isSaving}
                onChange={(next) => onContainerPruneStoppedDaysChange(toNumberText(next))}
                suffix={t('detail.retention.daysUnit')}
              />
            </div>
            <p className="text-xs text-muted-foreground">{t('detail.retention.containerPruneStoppedHint')}</p>
          </div>
        </SettingRow>

        <SettingRow
          title={t('detail.retention.autoUpdateTitle')}
          description={t('detail.retention.autoUpdateDescription')}
          checked={autoUpdateEnabled}
          disabled={isSaving}
          onCheckedChange={onAutoUpdateEnabledChange}
          statusOn={t('detail.retention.statusOn')}
          statusOff={t('detail.retention.statusOff')}
        >
          <AutoUpdateScheduleFields
            enabled={autoUpdateEnabled}
            isSaving={isSaving}
            windowStart={autoUpdateWindowStart}
            windowEnd={autoUpdateWindowEnd}
            daysSelected={autoUpdateDaysSelected}
            dayLabels={dayLabels}
            onWindowStartChange={onAutoUpdateWindowStartChange}
            onWindowEndChange={onAutoUpdateWindowEndChange}
            onDayToggle={onAutoUpdateDayToggle}
          />
        </SettingRow>

        <SettingRow
          title={t('detail.retention.metricRetentionTitle')}
          description={t('detail.retention.metricRetentionDescription')}
          checked={metricRetentionHours !== '0'}
          disabled={isSaving}
          onCheckedChange={(checked) => onMetricRetentionChange(checked ? '24' : '0')}
          statusOn={t('detail.retention.statusActive')}
          statusOff={t('detail.retention.statusForever')}
        >
          <div className="max-w-xs space-y-2">
            <label className="block text-xs font-semibold tracking-[0.14em] text-muted-foreground uppercase">
              {t('detail.retention.metricRetentionLabel')}
            </label>
            <div className="flex items-center gap-3">
              <NumberInput
                nullable
                min={0}
                max={8760}
                aria-label={t('detail.retention.metricRetentionLabel')}
                value={toNumberOrNull(metricRetentionHours)}
                disabled={metricRetentionHours === '0' || isSaving}
                onChange={(next) => onMetricRetentionChange(toNumberText(next))}
                suffix={t('detail.retention.hoursUnit')}
              />
            </div>
            <p className="text-xs text-muted-foreground">{t('detail.retention.metricRetentionHint')}</p>
          </div>
        </SettingRow>
      </CardContent>
    </Card>
  );
}
