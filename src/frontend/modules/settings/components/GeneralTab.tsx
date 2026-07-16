// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEraser } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { Spinner } from '@resources/components/ui/Spinner';
import { useAuth } from '@core/auth/useAuth';
import { useUpdatePreferences } from '@core/auth/usePreferences';
import { supportedLanguages, languageLabels, type SupportedLanguage } from '@core/i18n';
import { toast } from 'sonner';
import {
  useRetentionSettings,
  useSaveRetentionSettings,
  usePurgeAuditLog,
  usePurgeActivityLog,
  usePurgeLifecycleLog,
  usePurgeBackupLog,
  usePurgeScanHistory,
} from '../hooks/useRetentionSettings';
import { useSettings, useSaveSettings } from '../hooks/useSettings';
import type { AppSettings } from '../types';

type GeneralTabProps = {
  settings?: AppSettings;
};

type PurgeTarget = 'audit' | 'activity' | 'lifecycle' | 'backupLog' | 'scans' | null;

type RetentionRowProps = {
  days: number;
  onChange: (n: number) => void;
  clearDisabled: boolean;
  clearTooltip: string;
  onClear: () => void;
  pending: boolean;
};

function RetentionRow({
  days,
  onChange,
  clearDisabled,
  clearTooltip,
  onClear,
  pending,
}: RetentionRowProps) {
  const { t } = useTranslation('settings');
  return (
    <div className="flex flex-wrap items-center gap-3">
      <NumberInput value={days} onChange={onChange} min={0} max={3650} className="w-40" />
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={clearDisabled ? 0 : -1}>
            <Button
              type="button"
              variant="destructive"
              size="sm"
              onClick={onClear}
              disabled={clearDisabled}
              aria-label={t('retention.clearNow')}
            >
              <IconEraser className="size-4" />
              {t('retention.clearNow')}
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent>{clearTooltip}</TooltipContent>
      </Tooltip>
      {pending && <Spinner size="sm" className="text-muted-foreground" />}
    </div>
  );
}

type RetentionSectionProps = {
  labelKey: string;
  descriptionKey: string;
  days: number;
  onChange: (n: number) => void;
  clearDisabled: boolean;
  clearTooltip: string;
  onClear: () => void;
  pending: boolean;
};

function RetentionSection({
  labelKey,
  descriptionKey,
  days,
  onChange,
  clearDisabled,
  clearTooltip,
  onClear,
  pending,
}: RetentionSectionProps) {
  const { t } = useTranslation('settings');
  return (
    <div>
      <Label className="mb-2">{t(labelKey)}</Label>
      <p className="mb-2 text-sm text-muted-foreground">{t(descriptionKey)}</p>
      <RetentionRow
        days={days}
        onChange={onChange}
        clearDisabled={clearDisabled}
        clearTooltip={clearTooltip}
        onClear={onClear}
        pending={pending}
      />
    </div>
  );
}

// Map a "default language" key stored on the server (which can
// be any string) to a SupportedLanguage for the <Select> value.
// Unknown values fall back to "en".
function normalizeDefaultLanguage(value: string | undefined | null): SupportedLanguage {
  if (!value) return 'en';
  if ((supportedLanguages as readonly string[]).includes(value)) {
    return value as SupportedLanguage;
  }
  return 'en';
}

export function GeneralTab({ settings }: GeneralTabProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const user = useAuth((s) => s.user);
  const updatePreferences = useUpdatePreferences();
  const settingsQuery = useSettings();
  const saveSettings = useSaveSettings();
  const [refreshInterval, setRefreshInterval] = useState(
    settings?.autoRefreshInterval ?? settingsQuery.data?.autoRefreshInterval ?? 10,
  );

  // Admin-level app-wide default language. The backend stores it
  // as a plain settings row with key="default_language"; the
  // page-level admin-only <Select> writes it via the standard
  // /settings/{key} PUT endpoint.
  const [defaultLanguage, setDefaultLanguage] = useState<SupportedLanguage>(
    normalizeDefaultLanguage(settingsQuery.data?.defaultLanguage),
  );

  // Per-user UI preferences: time format (12h / 24h) and date
  // format (DD/MM/YYYY vs MM/DD/YYYY). These write to
  // /auth/preferences via the updatePreferences mutation, the same
  // endpoint that already handles preferredLanguage.
  const [timeFormat, setTimeFormat] = useState<'12h' | '24h'>(
    user?.timeFormat === '12h' ? '12h' : '24h',
  );
  const [dateFormat, setDateFormat] = useState<'ddmmyyyy' | 'mmddyyyy'>(
    user?.dateFormat === 'mmddyyyy' ? 'mmddyyyy' : 'ddmmyyyy',
  );
  const preferencesDirty =
    user?.timeFormat !== timeFormat || user?.dateFormat !== dateFormat;

  // Keep the local state in sync when the user record changes
  // (login, settings save, etc.) so the form reflects the latest
  // server value.
  useEffect(() => {
    if (user) {
      setTimeFormat(user.timeFormat === '12h' ? '12h' : '24h');
      setDateFormat(user.dateFormat === 'mmddyyyy' ? 'mmddyyyy' : 'ddmmyyyy');
    }
  }, [user?.timeFormat, user?.dateFormat]);

  // Re-sync the app-default language from the latest settings
  // query so the <Select> reflects server-side changes.
  useEffect(() => {
    const fromServer = settingsQuery.data?.defaultLanguage;
    if (fromServer) {
      setDefaultLanguage(normalizeDefaultLanguage(fromServer));
    }
  }, [settingsQuery.data?.defaultLanguage]);

  const { data: retention } = useRetentionSettings();
  const saveRetention = useSaveRetentionSettings();
  const [auditDays, setAuditDays] = useState(90);
  const [activityDays, setActivityDays] = useState(30);
  const [lifecycleDays, setLifecycleDays] = useState(30);
  const [backupLogDays, setBackupLogDays] = useState(30);
  const [scanDays, setScanDays] = useState(90);

  useEffect(() => {
    if (retention) {
      setAuditDays(retention.auditRetentionDays);
      setActivityDays(retention.activityRetentionDays);
      setLifecycleDays(retention.lifecycleRetentionDays);
      setBackupLogDays(retention.backupLogRetentionDays);
      setScanDays(retention.scanRetentionDays);
    }
  }, [retention]);

  const [purgeTarget, setPurgeTarget] = useState<PurgeTarget>(null);

  const purgeAudit = usePurgeAuditLog();
  const purgeActivity = usePurgeActivityLog();
  const purgeLifecycle = usePurgeLifecycleLog();
  const purgeBackupLog = usePurgeBackupLog();
  const purgeScans = usePurgeScanHistory();

  const auditRetentionDays = retention?.auditRetentionDays ?? 0;
  const activityRetentionDays = retention?.activityRetentionDays ?? 0;
  const lifecycleRetentionDays = retention?.lifecycleRetentionDays ?? 0;
  const backupLogRetentionDays = retention?.backupLogRetentionDays ?? 0;
  const scanRetentionDays = retention?.scanRetentionDays ?? 0;

  const auditEligible = auditRetentionDays > 0;
  const activityEligible = activityRetentionDays > 0;
  const lifecycleEligible = lifecycleRetentionDays > 0;
  const backupLogEligible = backupLogRetentionDays > 0;
  const scanEligible = scanRetentionDays > 0;

  const activeMutationPending = (() => {
    switch (purgeTarget) {
      case 'audit':
        return purgeAudit.isPending;
      case 'activity':
        return purgeActivity.isPending;
      case 'lifecycle':
        return purgeLifecycle.isPending;
      case 'backupLog':
        return purgeBackupLog.isPending;
      case 'scans':
        return purgeScans.isPending;
      default:
        return false;
    }
  })();

  const openConfirm = (target: Exclude<PurgeTarget, null>) => setPurgeTarget(target);
  const closeConfirm = () => {
    if (activeMutationPending) return;
    setPurgeTarget(null);
  };

  const confirmTitle = useMemo(() => {
    switch (purgeTarget) {
      case 'audit':
        return t('retention.clearAuditTitle');
      case 'activity':
        return t('retention.clearActivityTitle');
      case 'lifecycle':
        return t('retention.clearLifecycleTitle');
      case 'backupLog':
        return t('retention.clearBackupLogTitle');
      case 'scans':
        return t('retention.clearScansTitle');
      default:
        return '';
    }
  }, [purgeTarget, t]);

  const confirmDescription = useMemo(() => {
    switch (purgeTarget) {
      case 'audit':
        return t('retention.clearAuditDescription', { days: auditRetentionDays });
      case 'activity':
        return t('retention.clearActivityDescription', { days: activityRetentionDays });
      case 'lifecycle':
        return t('retention.clearLifecycleDescription', { days: lifecycleRetentionDays });
      case 'backupLog':
        return t('retention.clearBackupLogDescription', { days: backupLogRetentionDays });
      case 'scans':
        return t('retention.clearScansDescription', { days: scanRetentionDays });
      default:
        return '';
    }
  }, [purgeTarget, auditRetentionDays, activityRetentionDays, lifecycleRetentionDays, backupLogRetentionDays, scanRetentionDays, t]);

  const handleConfirm = () => {
    const onDone = () => setPurgeTarget(null);
    switch (purgeTarget) {
      case 'audit':
        purgeAudit.mutate(undefined, { onSuccess: onDone });
        break;
      case 'activity':
        purgeActivity.mutate(undefined, { onSuccess: onDone });
        break;
      case 'lifecycle':
        purgeLifecycle.mutate(undefined, { onSuccess: onDone });
        break;
      case 'backupLog':
        purgeBackupLog.mutate(undefined, { onSuccess: onDone });
        break;
      case 'scans':
        purgeScans.mutate(undefined, { onSuccess: onDone });
        break;
    }
  };

  const clearTooltip = (eligible: boolean, days: number) => {
    if (!eligible) return t('retention.keepForever');
    return t('retention.clearNowHint', { days });
  };

  async function handleSavePreferences() {
    if (!user) return;
    try {
      await updatePreferences.mutateAsync({
        timeFormat,
        dateFormat,
      });
      toast.success(t('general.preferencesSaved'));
    } catch {
      toast.error(t('general.preferencesSaveFailed'));
    }
  }

  async function handleSaveDefaultLanguage() {
    try {
      await saveSettings.mutateAsync({
        key: 'default_language',
        value: defaultLanguage,
        category: 'general',
      } as never);
      toast.success(t('general.defaultLanguageSaved'));
    } catch {
      toast.error(t('general.defaultLanguageSaveFailed'));
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <Label className="mb-2">{t('general.defaultApplicationLanguage')}</Label>
        <p className="mb-2 text-sm text-muted-foreground">
          {t('general.defaultApplicationLanguageDescription')}
        </p>
        <Select
          value={defaultLanguage}
          onChange={(v) => setDefaultLanguage(v as SupportedLanguage)}
          options={supportedLanguages.map((lang) => ({ value: lang, label: languageLabels[lang] }))}
          className="max-w-sm"
        />
        <Button
          onClick={handleSaveDefaultLanguage}
          className="ml-3"
          disabled={saveSettings.isPending}
        >
          {saveSettings.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      </div>

      {user && (
        <div>
          <Label className="mb-2">{t('general.userInterfacePreferences')}</Label>
          <p className="mb-2 text-sm text-muted-foreground">
            {t('general.userInterfacePreferencesDescription')}
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label className="mb-1.5 text-xs">
                {t('general.timeFormat')}
              </Label>
              <Select
                value={timeFormat}
                onChange={(v) => setTimeFormat(v === '12h' ? '12h' : '24h')}
                options={[
                  { value: '24h', label: t('general.timeFormat24h') },
                  { value: '12h', label: t('general.timeFormat12h') },
                ]}
                className="max-w-sm"
                ariaLabel={t('general.timeFormat')}
              />
            </div>
            <div>
              <Label className="mb-1.5 text-xs">
                {t('general.dateFormat')}
              </Label>
              <Select
                value={dateFormat}
                onChange={(v) => setDateFormat(v === 'mmddyyyy' ? 'mmddyyyy' : 'ddmmyyyy')}
                options={[
                  { value: 'ddmmyyyy', label: t('general.dateFormatDDMMYYYY') },
                  { value: 'mmddyyyy', label: t('general.dateFormatMMDDYYYY') },
                ]}
                className="max-w-sm"
                ariaLabel={t('general.dateFormat')}
              />
            </div>
          </div>
          <Button
            className="mt-3"
            onClick={handleSavePreferences}
            disabled={!preferencesDirty || updatePreferences.isPending}
          >
            {updatePreferences.isPending ? t('general.saving') : tc('actions.save')}
          </Button>
        </div>
      )}

      <div>
        <Label className="mb-2">{t('general.autoRefreshInterval')}</Label>
        <NumberInput
          value={refreshInterval}
          onChange={setRefreshInterval}
          min={1}
          max={300}
          className="w-40"
        />
        <Button
          onClick={() =>
            saveSettings.mutate({ autoRefreshInterval: refreshInterval } as never)
          }
          className="ml-3"
          disabled={saveSettings.isPending}
        >
          {saveSettings.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      </div>

      <hr className="border-border" />

      <div>
        <h3 className="text-sm font-semibold">{t('retention.title')}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{t('retention.description')}</p>
      </div>
      <RetentionSection
        labelKey="retention.auditDays"
        descriptionKey="retention.auditDaysDescription"
        days={auditDays}
        onChange={setAuditDays}
        clearDisabled={!auditEligible}
        clearTooltip={clearTooltip(auditEligible, auditRetentionDays)}
        onClear={() => openConfirm('audit')}
        pending={purgeAudit.isPending && purgeTarget !== 'audit'}
      />
      <RetentionSection
        labelKey="retention.activityDays"
        descriptionKey="retention.activityDaysDescription"
        days={activityDays}
        onChange={setActivityDays}
        clearDisabled={!activityEligible}
        clearTooltip={clearTooltip(activityEligible, activityRetentionDays)}
        onClear={() => openConfirm('activity')}
        pending={purgeActivity.isPending && purgeTarget !== 'activity'}
      />
      <RetentionSection
        labelKey="retention.lifecycleDays"
        descriptionKey="retention.lifecycleDaysDescription"
        days={lifecycleDays}
        onChange={setLifecycleDays}
        clearDisabled={!lifecycleEligible}
        clearTooltip={clearTooltip(lifecycleEligible, lifecycleRetentionDays)}
        onClear={() => openConfirm('lifecycle')}
        pending={purgeLifecycle.isPending && purgeTarget !== 'lifecycle'}
      />
      <RetentionSection
        labelKey="retention.backupLogDays"
        descriptionKey="retention.backupLogDaysDescription"
        days={backupLogDays}
        onChange={setBackupLogDays}
        clearDisabled={!backupLogEligible}
        clearTooltip={clearTooltip(backupLogEligible, backupLogRetentionDays)}
        onClear={() => openConfirm('backupLog')}
        pending={purgeBackupLog.isPending && purgeTarget !== 'backupLog'}
      />
      <RetentionSection
        labelKey="retention.scanDays"
        descriptionKey="retention.scanDaysDescription"
        days={scanDays}
        onChange={setScanDays}
        clearDisabled={!scanEligible}
        clearTooltip={clearTooltip(scanEligible, scanRetentionDays)}
        onClear={() => openConfirm('scans')}
        pending={purgeScans.isPending && purgeTarget !== 'scans'}
      />
      <Button
        onClick={() => saveRetention.mutate({
          auditRetentionDays: auditDays,
          activityRetentionDays: activityDays,
          lifecycleRetentionDays: lifecycleDays,
          backupLogRetentionDays: backupLogDays,
          scanRetentionDays: scanDays,
          backupRetentionCount: retention?.backupRetentionCount ?? 0,
          backupRetentionDays: retention?.backupRetentionDays ?? 0,
        })}
        disabled={saveRetention.isPending}
      >
        {saveRetention.isPending ? t('general.saving') : tc('actions.save')}
      </Button>

      <ConfirmDialog
        open={purgeTarget !== null}
        onOpenChange={(open) => {
          if (!open) closeConfirm();
        }}
        title={confirmTitle}
        description={confirmDescription}
        confirmLabel={t('retention.clearNow')}
        loading={activeMutationPending}
        onConfirm={handleConfirm}
      />
    </div>
  );
}
