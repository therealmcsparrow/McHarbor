// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@resources/components/ui/Card';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import type { EnvironmentInfo } from '../hooks/useEnvironments';
import { useTestEnvironment } from '../hooks/useEnvironmentActions';

function clampThreshold(value: number): number {
  if (!Number.isFinite(value)) {
    return 80;
  }

  return Math.min(100, Math.max(1, Math.round(value)));
}

type SettingRowProps = {
  title: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
  statusOn: string;
  statusOff: string;
  children?: ReactNode;
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

type EnvironmentActivityTabProps = {
  env: EnvironmentInfo;
  trackContainerEventsEnabled: boolean;
  collectContainerMetricsEnabled: boolean;
  highlightContainerChangesEnabled: boolean;
  dockerDiskUsageNotificationsEnabled: boolean;
  dockerDiskUsageThresholdPercent: string;
  isSaving: boolean;
  onTrackContainerEventsChange: (checked: boolean) => void;
  onCollectContainerMetricsChange: (checked: boolean) => void;
  onHighlightContainerChangesChange: (checked: boolean) => void;
  onDockerDiskUsageNotificationsChange: (checked: boolean) => void;
  onDockerDiskUsageThresholdChange: (value: string) => void;
};

export function EnvironmentActivityTab({
  env,
  trackContainerEventsEnabled,
  collectContainerMetricsEnabled,
  highlightContainerChangesEnabled,
  dockerDiskUsageNotificationsEnabled,
  dockerDiskUsageThresholdPercent,
  isSaving,
  onTrackContainerEventsChange,
  onCollectContainerMetricsChange,
  onHighlightContainerChangesChange,
  onDockerDiskUsageNotificationsChange,
  onDockerDiskUsageThresholdChange,
}: EnvironmentActivityTabProps) {
  const { t } = useTranslation('environments');
  const testEnvironment = useTestEnvironment();
  const [connectionTestState, setConnectionTestState] = useState<'idle' | 'success' | 'error'>('idle');

  useEffect(() => {
    setConnectionTestState('idle');
  }, [env.id]);

  const isDocker = env.orchestratorType === 'docker';
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
        {!isDocker && (
          <div className="rounded-xl border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
            {t('detail.activity.kubernetesNote')}
          </div>
        )}

        <SettingRow
          title={t('detail.activity.trackContainerEventsTitle')}
          description={t('detail.activity.trackContainerEventsDescription')}
          checked={trackContainerEventsEnabled}
          disabled={!isDocker || isSaving}
          onCheckedChange={onTrackContainerEventsChange}
          statusOn={t('detail.activity.statusOn')}
          statusOff={t('detail.activity.statusOff')}
        />

        <SettingRow
          title={t('detail.activity.collectContainerMetricsTitle')}
          description={t('detail.activity.collectContainerMetricsDescription')}
          checked={collectContainerMetricsEnabled}
          disabled={!isDocker || isSaving}
          onCheckedChange={onCollectContainerMetricsChange}
          statusOn={t('detail.activity.statusOn')}
          statusOff={t('detail.activity.statusOff')}
        />

        <SettingRow
          title={t('detail.activity.highlightContainerChangesTitle')}
          description={t('detail.activity.highlightContainerChangesDescription')}
          checked={highlightContainerChangesEnabled}
          disabled={!isDocker || isSaving}
          onCheckedChange={onHighlightContainerChangesChange}
          statusOn={t('detail.activity.statusOn')}
          statusOff={t('detail.activity.statusOff')}
        />

        <SettingRow
          title={t('detail.activity.diskUsageNotificationsTitle')}
          description={t('detail.activity.diskUsageNotificationsDescription')}
          checked={dockerDiskUsageNotificationsEnabled}
          disabled={!isDocker || isSaving}
          onCheckedChange={onDockerDiskUsageNotificationsChange}
          statusOn={t('detail.activity.statusOn')}
          statusOff={t('detail.activity.statusOff')}
        >
          <div className="max-w-xs space-y-2">
            <label className="block text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
              {t('detail.activity.diskUsageThresholdLabel')}
            </label>
            <div className="flex items-center gap-3">
              <Input
                type="number"
                min={1}
                max={100}
                value={dockerDiskUsageThresholdPercent}
                disabled={!isDocker || !dockerDiskUsageNotificationsEnabled || isSaving}
                onChange={(event) => onDockerDiskUsageThresholdChange(event.target.value)}
                onBlur={() => onDockerDiskUsageThresholdChange(String(normalizedThreshold))}
              />
              <span className="text-sm font-medium text-muted-foreground">%</span>
            </div>
            <p className="text-xs text-muted-foreground">{t('detail.activity.diskUsageThresholdHint')}</p>
            <p className="text-xs text-muted-foreground/80">{t('detail.activity.diskCapacityNote')}</p>
          </div>
        </SettingRow>
      </CardContent>
      <CardFooter className="flex items-center gap-3">
        <Button
          variant="outline"
          disabled={testEnvironment.isPending}
          onClick={() => {
            setConnectionTestState('idle');
            testEnvironment.mutate(env.id, {
              onSuccess: () => setConnectionTestState('success'),
              onError: () => setConnectionTestState('error'),
            });
          }}
        >
          {testEnvironment.isPending ? <Spinner size="sm" /> : null}
          {t('testConnection')}
        </Button>

        {testEnvironment.isPending || connectionTestState !== 'idle' ? (
          <div
            className={cn(
              'inline-flex size-9 items-center justify-center rounded-lg border transition-colors',
              testEnvironment.isPending && 'border-border bg-muted/30 text-muted-foreground',
              !testEnvironment.isPending && connectionTestState === 'success' && 'border-teal-500/30 bg-teal-500/10 text-teal-500',
              !testEnvironment.isPending && connectionTestState === 'error' && 'border-red-500/30 bg-red-500/10 text-red-500',
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

