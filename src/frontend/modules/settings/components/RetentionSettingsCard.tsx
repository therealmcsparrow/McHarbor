// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDatabaseCog } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Select } from '@resources/components/ui/Select';
import {
  useRetentionSettings,
  useSaveRetentionSettings,
  usePurgeAuditLog,
  usePurgeActivityLog,
  usePurgeLifecycleLog,
  usePurgeScanHistory,
  usePurgeAPIKeyUsage,
} from '../hooks/useRetentionSettings';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { PERM } from '@core/auth/permissionKeys';
import { RetentionLogRow } from './RetentionLogRow';
import { RetentionMetricsFields } from './RetentionMetricsFields';
import {
  daysFromRetention,
  DEFAULT_DAYS,
  DEFAULT_METRICS,
  DEFAULT_RECORD_EXEC,
  RETENTION_ROWS,
  type PurgeMutationLike,
  type RetentionKey,
} from './retention-rows';

/**
 * Data retention: how long each log is kept, plus a per-log "clear now"
 * escape hatch. Every day count saves together, so this group owns a
 * single Save in its footer.
 */
export function RetentionSettingsCard() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const { data: retention } = useRetentionSettings();
  const saveRetention = useSaveRetentionSettings();

  const [days, setDays] = useState<Record<RetentionKey, number>>(DEFAULT_DAYS);
  const [auditPreset, setAuditPreset] = useState('custom');
  const [purgeTarget, setPurgeTarget] = useState<RetentionKey | null>(null);
  const [metrics, setMetrics] = useState(DEFAULT_METRICS);
  const [recordExec, setRecordExec] = useState(DEFAULT_RECORD_EXEC);

  // Only what this card actually calls. Tying the map to one hook's return
  // type broke as soon as a purge reported a different payload — the API-key
  // clear returns two counts, not one — and the card never reads the payload.
  const purgeMutations: Record<RetentionKey, PurgeMutationLike> = {
    audit: usePurgeAuditLog(),
    activity: usePurgeActivityLog(),
    lifecycle: usePurgeLifecycleLog(),
    scans: usePurgeScanHistory(),
    apiKeyUsage: usePurgeAPIKeyUsage(),
  };

  const [syncedRetention, setSyncedRetention] = useState<typeof retention>(undefined);
  if (retention !== syncedRetention) {
    setSyncedRetention(retention);
    if (retention) {
      setDays(daysFromRetention(retention));
      setAuditPreset(retention.activeAuditRetentionPreset || 'custom');
      setMetrics({
        rawHours: retention.metricsRawHours,
        rollupDays: retention.metricsRollupDays,
        diskEnabled: retention.metricsDiskEnabled,
      });
      setRecordExec(retention.recordExecEvents);
    }
  }

  // What the server has, which is what a purge acts on — not the draft.
  const savedDays = daysFromRetention(retention);

  const auditRetentionPresets = retention?.auditRetentionPresets ?? [];
  const selectedAuditPreset = auditRetentionPresets.find((preset) => preset.id === auditPreset);
  const auditPresetOptions = auditRetentionPresets.map((preset) => ({
    value: preset.id,
    label: preset.isCustom
      ? t('retention.presetCustom')
      : t('retention.presetOption', { name: preset.name, days: preset.retentionDays }),
  }));
  const auditPresetLocksDays = selectedAuditPreset ? !selectedAuditPreset.isCustom : false;

  function handleAuditPresetChange(value: string) {
    setAuditPreset(value);
    const preset = auditRetentionPresets.find((item) => item.id === value);
    if (preset && !preset.isCustom) {
      setDays((prev) => ({ ...prev, audit: preset.retentionDays }));
    }
  }

  const activeMutationPending = purgeTarget ? purgeMutations[purgeTarget].isPending : false;
  const targetRow = RETENTION_ROWS.find((row) => row.key === purgeTarget);

  const confirmCopy =
    targetRow && purgeTarget
      ? {
          title: t(targetRow.confirmTitleKey),
          description: t(targetRow.confirmDescriptionKey, { days: savedDays[purgeTarget] }),
        }
      : { title: '', description: '' };

  function handleConfirm() {
    if (!purgeTarget) return;
    purgeMutations[purgeTarget].mutate(undefined, { onSuccess: () => setPurgeTarget(null) });
  }

  function handleSave() {
    saveRetention.mutate({
      auditRetentionDays: days.audit,
      activeAuditRetentionPreset: auditPreset,
      activityRetentionDays: days.activity,
      lifecycleRetentionDays: days.lifecycle,
      // Owned by the Backups page (BackupLogRetentionCard); echoed back
      // unchanged because /settings/retention is a whole-object PUT.
      backupLogRetentionDays: retention?.backupLogRetentionDays ?? 0,
      scanRetentionDays: days.scans,
      apiKeyUsageRetentionDays: days.apiKeyUsage,
      backupRetentionCount: retention?.backupRetentionCount ?? 0,
      backupRetentionDays: retention?.backupRetentionDays ?? 0,
      metricsRawHours: metrics.rawHours,
      metricsRollupDays: metrics.rollupDays,
      metricsDiskEnabled: metrics.diskEnabled,
      recordExecEvents: recordExec,
    });
  }

  return (
    <SettingsGroup
      icon={IconDatabaseCog}
      title={t('retention.title')}
      description={t('retention.description')}
      scope={t('general.scopeGlobal')}
      actions={
        <Button permission={PERM.SETTINGS_MANAGE} onClick={handleSave} disabled={saveRetention.isPending}>
          {saveRetention.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      }
    >
      <SettingsField
        label={t('retention.auditPreset')}
        description={selectedAuditPreset?.description || t('retention.auditPresetDescription')}
        className="rounded-lg border border-border bg-background/40 p-4"
      >
        <Select
          value={auditPreset}
          onChange={handleAuditPresetChange}
          options={auditPresetOptions}
          searchable={false}
          ariaLabel={t('retention.auditPreset')}
          className="max-w-xl"
        />
      </SettingsField>

      <div className="grid gap-5 lg:grid-cols-2">
        {RETENTION_ROWS.map((row) => (
          <RetentionLogRow
            key={row.key}
            labelKey={row.labelKey}
            descriptionKey={row.descriptionKey}
            days={days[row.key]}
            onDaysChange={(value) => setDays((prev) => ({ ...prev, [row.key]: value }))}
            savedDays={savedDays[row.key]}
            disabled={row.key === 'audit' && auditPresetLocksDays}
            purge={purgeMutations[row.key]}
            confirming={purgeTarget === row.key}
            onRequestPurge={() => setPurgeTarget(row.key)}
          />
        ))}
      </div>

      <RetentionMetricsFields
        metrics={metrics}
        onMetricsChange={setMetrics}
        recordExec={recordExec}
        onRecordExecChange={setRecordExec}
      />

      <ConfirmDialog
        open={purgeTarget !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen && !activeMutationPending) setPurgeTarget(null);
        }}
        title={confirmCopy.title}
        description={confirmCopy.description}
        confirmLabel={t('retention.clearNow')}
        loading={activeMutationPending}
        onConfirm={handleConfirm}
      />
    </SettingsGroup>
  );
}
