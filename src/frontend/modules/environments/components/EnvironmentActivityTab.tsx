// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import { SettingRow } from './ActivitySettingRow';
import type { EnvironmentInfo } from '../hooks/useEnvironments';
import { useTestEnvironment } from '../hooks/useEnvironmentActions';

function clampThreshold(value: number): number {
  if (!Number.isFinite(value)) {
    return 80;
  }

  return Math.min(100, Math.max(1, Math.round(value)));
}

type EnvironmentActivityTabProps = {
  env: EnvironmentInfo;
  trackContainerEventsEnabled: boolean;
  collectContainerMetricsEnabled: boolean;
  highlightContainerChangesEnabled: boolean;
  dockerDiskUsageNotificationsEnabled: boolean;
  dockerDiskUsageThresholdPercent: string;
  dockerDiskUsageUseGlobalDefault: boolean;
  globalDiskThresholdPercent: number;
  isSaving: boolean;
  onTrackContainerEventsChange: (checked: boolean) => void;
  onCollectContainerMetricsChange: (checked: boolean) => void;
  onHighlightContainerChangesChange: (checked: boolean) => void;
  onDockerDiskUsageNotificationsChange: (checked: boolean) => void;
  onDockerDiskUsageThresholdChange: (value: string) => void;
  onDockerDiskUsageUseGlobalDefaultChange: (checked: boolean) => void;
};

export function EnvironmentActivityTab({
  env,
  trackContainerEventsEnabled,
  collectContainerMetricsEnabled,
  highlightContainerChangesEnabled,
  dockerDiskUsageNotificationsEnabled,
  dockerDiskUsageThresholdPercent,
  dockerDiskUsageUseGlobalDefault,
  globalDiskThresholdPercent,
  isSaving,
  onTrackContainerEventsChange,
  onCollectContainerMetricsChange,
  onHighlightContainerChangesChange,
  onDockerDiskUsageNotificationsChange,
  onDockerDiskUsageThresholdChange,
  onDockerDiskUsageUseGlobalDefaultChange,
}: EnvironmentActivityTabProps) {
  const { t } = useTranslation('environments');
  const testEnvironment = useTestEnvironment();
  const { reset: resetConnectionTest } = testEnvironment;
  const connectionTestState: 'idle' | 'success' | 'error' = testEnvironment.isSuccess
    ? 'success'
    : testEnvironment.isError
      ? 'error'
      : 'idle';

  useEffect(() => {
    resetConnectionTest();
  }, [env.id, resetConnectionTest]);

  const normalizedThreshold = clampThreshold(Number.parseInt(dockerDiskUsageThresholdPercent, 10));
  const statusLabel =
    connectionTestState === 'success'
      ? t('detail.activity.connectionTestSuccessful')
      : t('detail.activity.connectionTestFailed');

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.activity.title')}</CardTitle>
        <CardDescription>{t('detail.activity.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <SettingRow
          title={t('detail.activity.trackContainerEventsTitle')}
          description={t('detail.activity.trackContainerEventsDescription')}
          checked={trackContainerEventsEnabled}
          disabled={isSaving}
          onCheckedChange={onTrackContainerEventsChange}
          statusOn={t('detail.activity.statusOn')}
          statusOff={t('detail.activity.statusOff')}
        />

        <SettingRow
          title={t('detail.activity.collectContainerMetricsTitle')}
          description={t('detail.activity.collectContainerMetricsDescription')}
          checked={collectContainerMetricsEnabled}
          disabled={isSaving}
          onCheckedChange={onCollectContainerMetricsChange}
          statusOn={t('detail.activity.statusOn')}
          statusOff={t('detail.activity.statusOff')}
        />

        <SettingRow
          title={t('detail.activity.highlightContainerChangesTitle')}
          description={t('detail.activity.highlightContainerChangesDescription')}
          checked={highlightContainerChangesEnabled}
          disabled={isSaving}
          onCheckedChange={onHighlightContainerChangesChange}
          statusOn={t('detail.activity.statusOn')}
          statusOff={t('detail.activity.statusOff')}
        />

        <SettingRow
          title={t('detail.activity.diskUsageNotificationsTitle')}
          description={t('detail.activity.diskUsageNotificationsDescription')}
          checked={dockerDiskUsageNotificationsEnabled}
          disabled={isSaving}
          onCheckedChange={onDockerDiskUsageNotificationsChange}
          statusOn={t('detail.activity.statusOn')}
          statusOff={t('detail.activity.statusOff')}
        >
          <div className="max-w-xs space-y-2">
            <label className="block text-xs font-semibold tracking-[0.14em] text-muted-foreground uppercase">
              {t('detail.activity.diskUsageThresholdLabel')}
            </label>
            <div className="flex items-center gap-3">
              <NumberInput
                nullable
                allowOutOfRange
                min={1}
                max={100}
                aria-label={t('detail.activity.diskUsageThresholdLabel')}
                value={
                  dockerDiskUsageUseGlobalDefault
                    ? globalDiskThresholdPercent
                    : toNumberOrNull(dockerDiskUsageThresholdPercent)
                }
                disabled={!dockerDiskUsageNotificationsEnabled || dockerDiskUsageUseGlobalDefault || isSaving}
                onChange={(next) => onDockerDiskUsageThresholdChange(toNumberText(next))}
                onBlur={() => onDockerDiskUsageThresholdChange(String(normalizedThreshold))}
                suffix="%"
              />
            </div>
            <p className="text-xs text-muted-foreground">{t('detail.activity.diskUsageThresholdHint')}</p>
            <div className="flex items-center gap-3 pt-1">
              <Switch
                checked={dockerDiskUsageUseGlobalDefault}
                disabled={!dockerDiskUsageNotificationsEnabled || isSaving}
                onCheckedChange={onDockerDiskUsageUseGlobalDefaultChange}
              />
              <span className="text-xs text-muted-foreground">
                {t('detail.activity.diskUsageUseGlobalDefaultLabel', { value: globalDiskThresholdPercent })}
              </span>
            </div>
            <p className="text-xs text-muted-foreground/80">{t('detail.activity.diskCapacityNote')}</p>
          </div>
        </SettingRow>
      </CardContent>
      <CardFooter className="flex items-center gap-3">
        <Button variant="outline" disabled={testEnvironment.isPending} onClick={() => testEnvironment.mutate(env.id)}>
          {testEnvironment.isPending ? <Spinner size="sm" /> : null}
          {t('testConnection')}
        </Button>

        {testEnvironment.isPending || connectionTestState !== 'idle' ? (
          <div
            className={cn(
              'inline-flex size-9 items-center justify-center rounded-lg border transition-colors',
              testEnvironment.isPending && 'border-border bg-muted/30 text-muted-foreground',
              !testEnvironment.isPending &&
                connectionTestState === 'success' &&
                'border-chart-1/30 bg-chart-1/10 text-chart-1',
              !testEnvironment.isPending &&
                connectionTestState === 'error' &&
                'border-danger/30 bg-danger/10 text-danger',
            )}
            aria-live="polite"
            aria-label={testEnvironment.isPending ? t('detail.activity.connectionTestInProgress') : statusLabel}
            title={testEnvironment.isPending ? t('detail.activity.connectionTestInProgress') : statusLabel}
          >
            {testEnvironment.isPending ? (
              <Spinner size="sm" />
            ) : connectionTestState === 'success' ? (
              <IconCheck className="size-4" />
            ) : (
              <IconX className="size-4" />
            )}
            <span className="sr-only">
              {testEnvironment.isPending ? t('detail.activity.connectionTestInProgress') : statusLabel}
            </span>
          </div>
        ) : null}
      </CardFooter>
    </Card>
  );
}
