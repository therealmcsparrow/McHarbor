// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDatabaseCog, IconEraser } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';
import { PERM } from '@core/auth/permissionKeys';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import {
  usePurgeBackupLog,
  useRetentionSettings,
  useSaveRetentionSettings,
} from '@resources/hooks/useRetentionSettings';

const DEFAULT_DAYS = 30;

/**
 * Backup log retention — the one retention horizon that belongs to backups
 * rather than to the app at large, so it sits on the Backups page next to
 * the log it prunes. The other four horizons stay in Settings → General.
 *
 * `/settings/retention` is a whole-object PUT, so the save spreads the
 * server's current values and overrides only this field: whatever the
 * General card holds locally is never clobbered from here.
 */
export function BackupLogRetentionCard() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const { data: retention } = useRetentionSettings();
  const saveRetention = useSaveRetentionSettings();
  const purgeBackupLog = usePurgeBackupLog();
  const [days, setDays] = useState(DEFAULT_DAYS);
  const [purgeOpen, setPurgeOpen] = useState(false);

  const [syncedRetention, setSyncedRetention] = useState<typeof retention>(undefined);
  if (retention && retention !== syncedRetention) {
    setSyncedRetention(retention);
    setDays(retention.backupLogRetentionDays);
  }

  const savedDays = retention?.backupLogRetentionDays ?? 0;
  // 0 days means "keep forever", and there is then nothing a purge would
  // delete — the button says so rather than silently doing nothing.
  const purgeable = savedDays > 0;

  function handleSave() {
    if (!retention) return;
    const { auditRetentionPresets: _presets, ...current } = retention;
    saveRetention.mutate({ ...current, backupLogRetentionDays: days });
  }

  return (
    <SettingsGroup
      icon={IconDatabaseCog}
      title={t('retention.backupSectionTitle')}
      description={t('retention.backupSectionDescription')}
      scope={t('general.scopeGlobal')}
      actions={
        <Button permission={PERM.SETTINGS_MANAGE} onClick={handleSave} disabled={saveRetention.isPending || !retention}>
          {saveRetention.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      }
    >
      <SettingsField
        htmlFor="settings-backup-log-retention-card-1"
        label={t('retention.backupLogDays')}
        description={t('retention.backupLogDaysDescription')}
      >
        <div className="flex flex-wrap items-center gap-3">
          <NumberInput
            id="settings-backup-log-retention-card-1"
            value={days}
            onChange={setDays}
            min={0}
            max={3650}
            className="w-40"
          />
          <Tooltip>
            <TooltipTrigger asChild>
              <TooltipFocusTarget tabIndex={!purgeable ? 0 : -1} className="inline">
                <Button
                  permission={PERM.SETTINGS_MANAGE}
                  type="button"
                  variant="destructive"
                  size="sm"
                  onClick={() => setPurgeOpen(true)}
                  disabled={!purgeable}
                  aria-label={t('retention.clearNow')}
                >
                  <IconEraser className="size-4" />
                  {t('retention.clearNow')}
                </Button>
              </TooltipFocusTarget>
            </TooltipTrigger>
            <TooltipContent>
              {purgeable ? t('retention.clearNowHint', { days: savedDays }) : t('retention.keepForever')}
            </TooltipContent>
          </Tooltip>
        </div>
      </SettingsField>

      <ConfirmDialog
        open={purgeOpen}
        onOpenChange={setPurgeOpen}
        title={t('retention.clearBackupLogTitle')}
        description={t('retention.clearBackupLogDescription', { days: savedDays })}
        confirmLabel={t('retention.clearNow')}
        loading={purgeBackupLog.isPending}
        onConfirm={() => purgeBackupLog.mutate(undefined, { onSuccess: () => setPurgeOpen(false) })}
      />
    </SettingsGroup>
  );
}
