// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconClock, IconDatabaseExport } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { PERM } from '@core/auth/permissionKeys';
import { BackupNameField, BackupSelectionFields } from '../BackupSelectionFields';
import { BackupConsistencyField } from '../BackupConsistencyField';
import { hasDestination } from '../plan-utils';
import type { ConfigSection, ContainerBackupInput, ContainerBackupOption } from '@resources/hooks/useContainerBackups';
import type { StorageLocation } from '@resources/hooks/useStorageLocations';

type BackupSettingsPanelProps = {
  value: ContainerBackupInput;
  options: ContainerBackupOption[];
  storageLocations: StorageLocation[];
  containerId: string;
  configSections: ConfigSection[];
  containerInspect?: Record<string, unknown>;
  fullBackup: boolean;
  onFullBackupChange: (value: boolean) => void;
  // scheduled turns this group from a one-off backup into a recurring
  // plan: it reveals the cron / retention / log-tail fields and adds the
  // "Add schedule" action. It is UI state only — the payload already
  // carries `cron`, and the ad-hoc run endpoint ignores it.
  scheduled: boolean;
  onScheduledChange: (value: boolean) => void;
  onChange: (patch: Partial<ContainerBackupInput>) => void;
  onRunNow: () => void;
  onSaveSchedule: () => void;
  runPending: boolean;
  runDisabled: boolean;
  savePending: boolean;
};

export function BackupSettingsPanel({
  value,
  options,
  storageLocations,
  containerId,
  configSections,
  containerInspect,
  fullBackup,
  onFullBackupChange,
  scheduled,
  onScheduledChange,
  onChange,
  onRunNow,
  onSaveSchedule,
  runPending,
  runDisabled,
  savePending,
}: BackupSettingsPanelProps) {
  const { t } = useTranslation('containers');
  const nameMissing = value.name.trim() === '';
  const cronMissing = !value.cron?.trim();
  // A backup with nowhere to go is rejected by the backend, so the
  // action is disabled rather than left to fail with a toast.
  const noDestination = !hasDestination(value);

  return (
    <section className="mc-glass rounded-lg p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-foreground">{t('backups.title')}</h2>
          <p className="text-xs text-muted-foreground">{t('backups.description')}</p>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-3">
          <label className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{t('backups.scheduleToggle')}</span>
            <Switch checked={scheduled} onCheckedChange={onScheduledChange} aria-label={t('backups.scheduleToggle')} />
          </label>
          {scheduled && (
            <>
              <label className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">{t('backups.enabled')}</span>
                <Switch
                  checked={value.enabled ?? true}
                  onCheckedChange={(enabled) => onChange({ enabled })}
                  aria-label={t('backups.enabled')}
                />
              </label>
              <Button
                permission={PERM.BACKUPS_MANAGE}
                onClick={onSaveSchedule}
                disabled={savePending || nameMissing || cronMissing || noDestination}
              >
                <IconClock className="size-4" />
                {savePending ? t('backups.saving') : t('backups.saveSchedule')}
              </Button>
            </>
          )}
          <Button
            variant={scheduled ? 'outline' : 'default'}
            permission={PERM.BACKUPS_MANAGE}
            onClick={onRunNow}
            disabled={runPending || runDisabled || nameMissing || noDestination}
          >
            <IconDatabaseExport className="size-4" />
            {runPending || runDisabled ? t('backups.running') : t('backups.runNow')}
          </Button>
        </div>
      </div>

      <div className="mt-4">
        <BackupNameField value={value} onChange={onChange} />
      </div>

      {noDestination && (
        <p className="mt-3 rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive">
          {t('backups.destinationRequired')}
        </p>
      )}
      {scheduled && (
        <p className="mt-3 rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
          {t('backups.scheduleToggleHint')}
        </p>
      )}

      <div className="mt-4 rounded-lg border border-border p-3">
        <BackupConsistencyField value={value} onChange={onChange} />
      </div>

      <div className="mt-4">
        <BackupSelectionFields
          value={value}
          options={options}
          storageLocations={storageLocations}
          containerId={containerId}
          configSections={configSections}
          containerInspect={containerInspect}
          showCron={scheduled}
          fullBackup={fullBackup}
          onFullBackupChange={onFullBackupChange}
          showName={false}
          onChange={onChange}
        />
      </div>
    </section>
  );
}
