// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import type { ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import { PERM } from '@core/auth/permissionKeys';
import { RestoreLayerSection } from './RestoreLayerSection';
import type { RestoreRunState } from './useRestoreRunState';

type RestoreOptionsPanelProps = {
  run: ContainerBackupRun | null;
  state: RestoreRunState;
};

// RestoreOptionsPanel renders the "what to restore" choices: the permission,
// key and loading states, then one layer card per filesystem, config and mount.
export function RestoreOptionsPanel({ run, state }: RestoreOptionsPanelProps) {
  const { t } = useTranslation('containers');
  const choiceId = useId();
  const runId = run?.id ?? '';
  const {
    restoreOptions,
    filesystemItem,
    configItem,
    mountItems,
    imageAndLogItems,
    modeFor,
    setModeFor,
    restoreSecretKey,
    restoreSelectedItems,
    setRestoreSelectedItems,
  } = state;

  return (
    <div className="space-y-3">
      <p className="text-xs font-medium text-muted-foreground uppercase">{t('backups.restoreChoices')}</p>
      {!state.canViewRestoreOptions && (
        <Alert
          variant="warning"
          title={t('permission.denied', { ns: 'common' })}
          description={t('permission.requires', { ns: 'common', permission: PERM.RESTORE_VIEW })}
        />
      )}
      {state.canViewRestoreOptions && !state.restoreOptionsEnabled && (
        <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
          {t('backups.restoreChoicesNeedKey')}
        </p>
      )}
      {state.restoreOptionsEnabled && state.restoreOptionsLoading && (
        <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
          <Spinner size="sm" />
          {t('backups.restoreChoicesLoading')}
        </div>
      )}
      {state.restoreOptionsError && (
        <p className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
          {t('backups.restoreChoicesFailed')}
        </p>
      )}
      {restoreOptions && restoreOptions.items.length === 0 && (
        <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
          {t('backups.restoreChoicesEmpty')}
        </p>
      )}
      {restoreOptions && restoreOptions.items.length > 0 && (
        <div className="space-y-3 rounded-lg border border-border bg-background p-3">
          {filesystemItem && (
            <RestoreLayerSection
              title={t('backups.restoreFilesystemTitle')}
              description={t('backups.restoreFilesystemDescription')}
              toggleLabel={t('backups.restoreLayerFilesystem')}
              hintLayer={t('backups.restoreLayerFilesystem')}
              mode={modeFor('filesystem')}
              onModeChange={(mode) => setModeFor('filesystem', mode)}
              runId={runId}
              mountTarget="filesystem"
              kind="filesystem"
              secretKey={restoreSecretKey}
              selected={restoreSelectedItems}
              onSelectedChange={setRestoreSelectedItems}
            />
          )}

          {configItem && (
            <RestoreLayerSection
              title={t('backups.restoreConfigTitle')}
              description={t('backups.restoreConfigDescription')}
              toggleLabel={t('backups.restoreLayerConfig')}
              hintLayer={t('backups.restoreLayerConfig')}
              mode={modeFor('config')}
              onModeChange={(mode) => setModeFor('config', mode)}
              runId={runId}
              mountTarget="config"
              kind="config"
              secretKey={restoreSecretKey}
              selected={restoreSelectedItems}
              onSelectedChange={setRestoreSelectedItems}
            />
          )}
          {configItem && modeFor('config') !== 'off' && (
            <Alert
              variant="warning"
              title={t('backups.restoreConfigRecreateTitle')}
              description={t('backups.restoreConfigRecreate')}
            />
          )}

          {mountItems.map((mount) => (
            <RestoreLayerSection
              key={mount.key}
              title={mount.label}
              description={mount.description || undefined}
              toggleLabel={mount.label}
              hintLayer={t('backups.restoreLayerMount')}
              mode={modeFor(mount.key)}
              onModeChange={(mode) => setModeFor(mount.key, mode)}
              runId={runId}
              // RestoreFileTree forwards mountTarget to the backend mount-entry
              // endpoint, which derives "mounts/<safe>.tar" from it, so the
              // synthetic "mount:" selection prefix must not reach it or the
              // endpoint 404s.
              mountTarget={mount.key.startsWith('mount:') ? mount.key.slice('mount:'.length) : mount.key}
              kind="mount"
              secretKey={restoreSecretKey}
              selected={restoreSelectedItems}
              onSelectedChange={setRestoreSelectedItems}
            />
          ))}

          {imageAndLogItems.length > 0 && (
            <div className="space-y-1 rounded-md border border-border p-3">
              <p className="text-xs font-medium text-muted-foreground uppercase">{t('backups.restoreChoices')}</p>
              {imageAndLogItems.map((item) => (
                <label
                  key={item.key}
                  htmlFor={`${choiceId}-${item.key}`}
                  className="flex items-start gap-3 rounded-md p-2 transition-colors hover:bg-muted/50"
                >
                  <Switch
                    id={`${choiceId}-${item.key}`}
                    checked={restoreSelectedItems.includes(item.key)}
                    disabled={item.required || state.restoreBackup.isPending}
                    onCheckedChange={(checked) => state.toggleRestoreItem(item.key, checked)}
                    aria-label={item.label}
                    className="mt-0.5"
                  />
                  <span className="min-w-0">
                    <span className="block text-sm font-medium text-foreground">{item.label}</span>
                    <span className="block text-xs text-muted-foreground">{item.description}</span>
                  </span>
                </label>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
