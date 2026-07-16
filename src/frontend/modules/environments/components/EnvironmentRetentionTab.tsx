// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@resources/components/ui/Card';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import type { AutoUpdateDay } from '../hooks/useEnvironmentDetailState';
import type { EnvironmentInfo } from '../hooks/useEnvironments';

const ALL_DAYS = [
  { value: 'mon', short: 'Mon' },
  { value: 'tue', short: 'Tue' },
  { value: 'wed', short: 'Wed' },
  { value: 'thu', short: 'Thu' },
  { value: 'fri', short: 'Fri' },
  { value: 'sat', short: 'Sat' },
  { value: 'sun', short: 'Sun' },
] as const;

type DayChipProps = {
  day: (typeof ALL_DAYS)[number];
  selected: boolean;
  disabled: boolean;
  onToggle: (value: string) => void;
  labels: Record<string, string>;
};

function DayChip({ day, selected, disabled, onToggle, labels }: DayChipProps) {
  return (
    <Button
      type="button"
      variant={selected ? 'default' : 'outline'}
      size="sm"
      onClick={() => onToggle(day.value)}
      disabled={disabled}
      aria-pressed={selected}
      className={cn(
        'inline-flex h-9 min-w-[3rem] items-center justify-center rounded-lg border px-3 text-xs font-semibold uppercase tracking-[0.12em] transition-colors',
        selected
          ? 'border-teal-500/40 bg-teal-500/15 text-teal-500'
          : 'border-border bg-card/40 text-muted-foreground hover:border-border/80 hover:text-foreground',
        disabled && 'cursor-not-allowed opacity-60',
      )}
    >
      {labels[day.value] ?? day.short}
    </Button>
  );
}

type SettingRowProps = {
  title: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
  statusOn: string;
  statusOff: string;
  children?: React.ReactNode;
};

function SettingRow({
  title,
  description,
  checked,
  disabled = false,
  onCheckedChange,
  statusOn,
  statusOff,
  children,
}: SettingRowProps) {
  return (
    <div className="space-y-4 rounded-xl border border-border bg-muted/20 p-4">
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <h3 className="text-sm font-semibold text-foreground">{title}</h3>
            <span className="text-[11px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              {checked ? statusOn : statusOff}
            </span>
          </div>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>

        <Switch checked={checked} disabled={disabled} onCheckedChange={onCheckedChange} />
      </div>

      {children}
    </div>
  );
}

type EnvironmentRetentionTabProps = {
  env: EnvironmentInfo;
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
  env,
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

  const isDocker = env.orchestratorType === 'docker';
  const dayLabels: Record<string, string> = t('detail.retention.daysShort', { returnObjects: true }) as Record<string, string>;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.retention.title')}</CardTitle>
        <CardDescription>{t('detail.retention.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!isDocker && (
          <div className="rounded-xl border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
            {t('detail.retention.kubernetesNote')}
          </div>
        )}

        <SettingRow
          title={t('detail.retention.logRetentionTitle')}
          description={t('detail.retention.logRetentionDescription')}
          checked={logRetentionDays !== '0'}
          disabled={!isDocker || isSaving}
          onCheckedChange={(checked) => onLogRetentionChange(checked ? '7' : '0')}
          statusOn={t('detail.retention.statusActive')}
          statusOff={t('detail.retention.statusForever')}
        >
          <div className="max-w-xs space-y-2">
            <label className="block text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
              {t('detail.retention.logRetentionLabel')}
            </label>
            <div className="flex items-center gap-3">
              <Input
                type="number"
                min={0}
                max={3650}
                value={logRetentionDays}
                disabled={!isDocker || logRetentionDays === '0' || isSaving}
                onChange={(event) => onLogRetentionChange(event.target.value)}
              />
              <span className="text-sm font-medium text-muted-foreground">
                {t('detail.retention.daysUnit')}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">{t('detail.retention.logRetentionHint')}</p>
          </div>
        </SettingRow>

        <SettingRow
          title={t('detail.retention.containerPruneTitle')}
          description={t('detail.retention.containerPruneDescription')}
          checked={containerPruneEnabled}
          disabled={!isDocker || isSaving}
          onCheckedChange={onContainerPruneEnabledChange}
          statusOn={t('detail.retention.statusOn')}
          statusOff={t('detail.retention.statusOff')}
        >
          <div className="max-w-xs space-y-2">
            <label className="block text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
              {t('detail.retention.containerPruneStoppedLabel')}
            </label>
            <div className="flex items-center gap-3">
              <Input
                type="number"
                min={1}
                max={3650}
                value={containerPruneStoppedDays}
                disabled={!isDocker || !containerPruneEnabled || isSaving}
                onChange={(event) => onContainerPruneStoppedDaysChange(event.target.value)}
              />
              <span className="text-sm font-medium text-muted-foreground">
                {t('detail.retention.daysUnit')}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {t('detail.retention.containerPruneStoppedHint')}
            </p>
          </div>
        </SettingRow>

        <SettingRow
          title={t('detail.retention.autoUpdateTitle')}
          description={t('detail.retention.autoUpdateDescription')}
          checked={autoUpdateEnabled}
          disabled={!isDocker || isSaving}
          onCheckedChange={onAutoUpdateEnabledChange}
          statusOn={t('detail.retention.statusOn')}
          statusOff={t('detail.retention.statusOff')}
        >
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <label className="block text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                  {t('detail.retention.autoUpdateWindowStartLabel')}
                </label>
                <Input
                  type="time"
                  value={autoUpdateWindowStart}
                  disabled={!isDocker || !autoUpdateEnabled || isSaving}
                  onChange={(event) => onAutoUpdateWindowStartChange(event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <label className="block text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                  {t('detail.retention.autoUpdateWindowEndLabel')}
                </label>
                <Input
                  type="time"
                  value={autoUpdateWindowEnd}
                  disabled={!isDocker || !autoUpdateEnabled || isSaving}
                  onChange={(event) => onAutoUpdateWindowEndChange(event.target.value)}
                />
              </div>
            </div>
            <div className="space-y-2">
              <label className="block text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                {t('detail.retention.autoUpdateDaysLabel')}
              </label>
              <div className="flex flex-wrap gap-2">
                {ALL_DAYS.map((day) => (
                  <DayChip
                    key={day.value}
                    day={day}
                    selected={autoUpdateDaysSelected.has(day.value)}
                    disabled={!isDocker || !autoUpdateEnabled || isSaving}
                    onToggle={onAutoUpdateDayToggle}
                    labels={dayLabels}
                  />
                ))}
              </div>
            </div>
          </div>
        </SettingRow>

        <SettingRow
          title={t('detail.retention.metricRetentionTitle')}
          description={t('detail.retention.metricRetentionDescription')}
          checked={metricRetentionHours !== '0'}
          disabled={!isDocker || isSaving}
          onCheckedChange={(checked) => onMetricRetentionChange(checked ? '24' : '0')}
          statusOn={t('detail.retention.statusActive')}
          statusOff={t('detail.retention.statusForever')}
        >
          <div className="max-w-xs space-y-2">
            <label className="block text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
              {t('detail.retention.metricRetentionLabel')}
            </label>
            <div className="flex items-center gap-3">
              <Input
                type="number"
                min={0}
                max={8760}
                value={metricRetentionHours}
                disabled={!isDocker || metricRetentionHours === '0' || isSaving}
                onChange={(event) => onMetricRetentionChange(event.target.value)}
              />
              <span className="text-sm font-medium text-muted-foreground">
                {t('detail.retention.hoursUnit')}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">{t('detail.retention.metricRetentionHint')}</p>
          </div>
        </SettingRow>
      </CardContent>
    </Card>
  );
}
