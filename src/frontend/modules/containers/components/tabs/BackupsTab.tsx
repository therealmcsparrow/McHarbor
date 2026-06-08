// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconClock, IconDatabaseExport, IconDownload, IconPlayerPlay, IconRestore, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { describeCron } from '@resources/utils/schedule';
import { useStorageLocations } from '../../../settings/hooks/useStorageLocations';
import { BackupSelectionFields } from '../BackupSelectionFields';
import {
  type ContainerBackupInput,
  type ContainerBackupOption,
  type ContainerBackupPlan,
  type ContainerBackupRun,
  containerBackupDownloadUrl,
  useContainerBackupOptions,
  useContainerBackupPlans,
  useContainerBackupRuns,
  useCreateContainerBackupPlan,
  useDeleteContainerBackupPlan,
  useRestoreContainerBackup,
  useRunContainerBackup,
  useRunContainerBackupPlan,
} from '../../hooks/useContainerBackups';

type BackupsTabProps = {
  containerId: string;
  containerName: string;
};

const DEFAULT_INPUT: ContainerBackupInput = {
  name: '',
  storageLocationId: '',
  storageLocationIds: [],
  includeConfig: true,
  includeLogs: false,
  includeFilesystem: false,
  includeImage: false,
  selectedMounts: [],
  cron: '',
  enabled: true,
  retentionCount: 0,
  retentionDays: 0,
};

function defaultInput(options: ContainerBackupOption[], containerName: string): ContainerBackupInput {
  return {
    ...DEFAULT_INPUT,
    name: `${containerName} backup`,
    includeLogs: options.some((option) => option.type === 'logs' && option.default),
    includeFilesystem: options.some((option) => option.type === 'filesystem' && option.default),
    includeImage: options.some((option) => option.type === 'image' && option.default),
    selectedMounts: options
      .filter((option) => option.key.startsWith('mount:') && option.default)
      .map((option) => option.key.slice('mount:'.length)),
  };
}

function formatBytes(value: number) {
  if (!value) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function retentionSummary(plan: ContainerBackupPlan, t: (key: string, options?: Record<string, number>) => string) {
  if (plan.retentionCount > 0 && plan.retentionDays > 0) {
    return t('backups.retentionCountDays', { count: plan.retentionCount, days: plan.retentionDays });
  }
  if (plan.retentionCount > 0) {
    return t('backups.retentionCountOnly', { count: plan.retentionCount });
  }
  if (plan.retentionDays > 0) {
    return t('backups.retentionDaysOnly', { days: plan.retentionDays });
  }
  return t('backups.retentionForever');
}

export function BackupsTab({ containerId, containerName }: BackupsTabProps) {
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const { data: optionData, isLoading: optionsLoading } = useContainerBackupOptions(containerId);
  const { data: plans = [], isLoading: plansLoading } = useContainerBackupPlans(containerId);
  const { data: runs = [], isLoading: runsLoading } = useContainerBackupRuns(containerId);
  const { data: storageLocations = [] } = useStorageLocations();
  const runManual = useRunContainerBackup(containerId);
  const createPlan = useCreateContainerBackupPlan(containerId);
  const runPlan = useRunContainerBackupPlan();
  const deletePlan = useDeleteContainerBackupPlan();
  const restoreBackup = useRestoreContainerBackup();
  const [manualInput, setManualInput] = useState<ContainerBackupInput>(DEFAULT_INPUT);
  const [scheduleInput, setScheduleInput] = useState<ContainerBackupInput>({ ...DEFAULT_INPUT, cron: '0 3 * * *' });
  const [deleteTarget, setDeleteTarget] = useState<ContainerBackupPlan | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<ContainerBackupRun | null>(null);
  const [restoreSecretTarget, setRestoreSecretTarget] = useState<ContainerBackupRun | null>(null);
  const [restoreSecretKey, setRestoreSecretKey] = useState('');

  const options = useMemo(() => optionData?.options ?? [], [optionData?.options]);

  useEffect(() => {
    if (options.length === 0) return;
    const next = defaultInput(options, containerName);
    setManualInput(next);
    setScheduleInput({ ...next, cron: '0 3 * * *', enabled: true });
  }, [containerName, options]);

  if (optionsLoading || plansLoading || runsLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  function handleRestore(run: ContainerBackupRun) {
    if (run.requiresSecretKey) {
      setRestoreSecretKey('');
      setRestoreSecretTarget(run);
      return;
    }
    setRestoreTarget(run);
  }

  function restoreWithCurrentKey() {
    if (!restoreTarget) return;
    restoreBackup.mutate({ id: restoreTarget.id }, { onSuccess: () => setRestoreTarget(null) });
  }

  function restoreWithSecretKey() {
    if (!restoreSecretTarget) return;
    restoreBackup.mutate(
      { id: restoreSecretTarget.id, secretKey: restoreSecretKey },
      {
        onSuccess: () => {
          setRestoreSecretTarget(null);
          setRestoreSecretKey('');
        },
      },
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(360px,0.8fr)]">
      <div className="space-y-4">
        <section className="rounded-lg border border-border bg-card p-4">
          <div className="mb-4 flex items-center justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold text-foreground">{t('backups.manualTitle')}</h2>
              <p className="text-xs text-muted-foreground">{t('backups.manualDescription')}</p>
            </div>
            <Button
              onClick={() => runManual.mutate(manualInput)}
              disabled={runManual.isPending || manualInput.name.trim() === ''}
            >
              <IconDatabaseExport className="size-4" />
              {runManual.isPending ? t('backups.running') : t('backups.runNow')}
            </Button>
          </div>
          <BackupSelectionFields
            value={manualInput}
            options={options}
            storageLocations={storageLocations}
            onChange={(patch) => setManualInput((current) => ({ ...current, ...patch }))}
          />
        </section>

        <section className="rounded-lg border border-border bg-card p-4">
          <div className="mb-4 flex items-center justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold text-foreground">{t('backups.scheduleTitle')}</h2>
              <p className="text-xs text-muted-foreground">{t('backups.scheduleDescription')}</p>
            </div>
            <div className="flex items-center gap-3">
              <span className="text-xs text-muted-foreground">{t('backups.enabled')}</span>
              <Switch
                checked={scheduleInput.enabled ?? true}
                onCheckedChange={(enabled) => setScheduleInput((current) => ({ ...current, enabled }))}
                aria-label={t('backups.enabled')}
              />
              <Button
                onClick={() => createPlan.mutate(scheduleInput)}
                disabled={createPlan.isPending || scheduleInput.name.trim() === '' || !scheduleInput.cron?.trim()}
              >
                <IconClock className="size-4" />
                {createPlan.isPending ? t('backups.saving') : t('backups.saveSchedule')}
              </Button>
            </div>
          </div>
          <BackupSelectionFields
            value={scheduleInput}
            options={options}
            storageLocations={storageLocations}
            showCron
            onChange={(patch) => setScheduleInput((current) => ({ ...current, ...patch }))}
          />
        </section>
      </div>

      <div className="space-y-4">
        <section className="rounded-lg border border-border bg-card p-4">
          <h2 className="mb-3 text-sm font-semibold text-foreground">{t('backups.savedSchedules')}</h2>
          {plans.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">{t('backups.noSchedules')}</p>
          ) : (
            <div className="space-y-2">
              {plans.map((plan) => (
                <div key={plan.id} className="rounded-lg border border-border bg-background p-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <h3 className="truncate text-sm font-medium text-foreground">{plan.name}</h3>
                        <Badge variant={plan.enabled ? 'default' : 'secondary'}>
                          {plan.enabled ? t('backups.enabled') : t('backups.disabled')}
                        </Badge>
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">{describeCron(plan.cron, tc)}</p>
                      {plan.storageLocationIds.length > 0 && (
                        <p className="text-xs text-muted-foreground">
                          {t('backups.destinationsCount', { count: plan.storageLocationIds.length })}
                        </p>
                      )}
                      <p className="text-xs text-muted-foreground">{retentionSummary(plan, t)}</p>
                      {plan.nextRunAt && (
                        <p className="text-xs text-muted-foreground">{t('backups.nextRun', { value: new Date(plan.nextRunAt).toLocaleString() })}</p>
                      )}
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t('backups.runNow')}
                        onClick={() => runPlan.mutate(plan.id)}
                        disabled={runPlan.isPending}
                      >
                        <IconPlayerPlay className="size-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t('backups.deleteSchedule')}
                        onClick={() => setDeleteTarget(plan)}
                      >
                        <IconTrash className="size-4 text-destructive" />
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>

        <section className="rounded-lg border border-border bg-card p-4">
          <h2 className="mb-3 text-sm font-semibold text-foreground">{t('backups.recentRuns')}</h2>
          {runs.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">{t('backups.noRuns')}</p>
          ) : (
            <div className="space-y-2">
              {runs.map((run) => (
                <div key={run.id} className="rounded-lg border border-border bg-background p-3">
                  <div className="flex items-center justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <Badge variant={run.status === 'success' ? 'success' : run.status === 'failure' ? 'destructive' : 'secondary'}>
                          {t(`backups.status.${run.status}`)}
                        </Badge>
                        <span className="text-xs text-muted-foreground">{new Date(run.startedAt).toLocaleString()}</span>
                      </div>
                      {run.archivePath && <p className="mt-1 truncate font-mono text-xs text-muted-foreground">{run.archivePath}</p>}
                      {run.error && <p className="mt-1 text-xs text-destructive">{run.error}</p>}
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <span className="text-xs text-muted-foreground">{formatBytes(run.archiveSize)}</span>
                      {run.status === 'success' && run.archivePath && (
                        <>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={t('backups.restore')}
                            onClick={() => handleRestore(run)}
                            disabled={restoreBackup.isPending}
                          >
                            <IconRestore className="size-4" />
                          </Button>
                          <Button variant="ghost" size="icon-sm" asChild aria-label={t('backups.download')}>
                            <a href={containerBackupDownloadUrl(run.id)} download>
                              <IconDownload className="size-4" />
                            </a>
                          </Button>
                        </>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>

      {deleteTarget && (
        <ConfirmDialog
          open={!!deleteTarget}
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t('backups.deleteScheduleTitle')}
          description={t('backups.deleteScheduleDescription')}
          confirmLabel={t('backups.deleteSchedule')}
          loading={deletePlan.isPending}
          onConfirm={() => {
            deletePlan.mutate(deleteTarget.id, { onSuccess: () => setDeleteTarget(null) });
          }}
        />
      )}
      {restoreTarget && (
        <ConfirmDialog
          open={!!restoreTarget}
          onOpenChange={(open) => {
            if (!open) setRestoreTarget(null);
          }}
          title={t('backups.restoreTitle')}
          description={t('backups.restoreDescription')}
          confirmLabel={t('backups.restore')}
          loading={restoreBackup.isPending}
          onConfirm={restoreWithCurrentKey}
        />
      )}
      <Dialog
        open={!!restoreSecretTarget}
        onOpenChange={(open) => {
          if (!open) {
            setRestoreSecretTarget(null);
            setRestoreSecretKey('');
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('backups.restoreSecretTitle')}</DialogTitle>
            <DialogDescription>{t('backups.restoreSecretDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-3">
            <Input
              type="password"
              value={restoreSecretKey}
              onChange={(event) => setRestoreSecretKey(event.target.value)}
              placeholder={t('backups.restoreSecretPlaceholder')}
              autoComplete="off"
            />
            <p className="text-xs text-muted-foreground">{t('backups.restoreWarning')}</p>
          </DialogBody>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setRestoreSecretTarget(null);
                setRestoreSecretKey('');
              }}
            >
              {t('backups.cancelRestore')}
            </Button>
            <Button
              variant="destructive"
              onClick={restoreWithSecretKey}
              disabled={restoreBackup.isPending || restoreSecretKey.trim() === ''}
            >
              <IconRestore className="size-4" />
              {restoreBackup.isPending ? t('backups.restoring') : t('backups.restore')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
