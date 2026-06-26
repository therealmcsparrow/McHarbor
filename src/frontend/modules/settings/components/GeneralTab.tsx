// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEraser } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { useLanguageStore } from '@resources/stores/language';
import { useAuth } from '@core/auth/useAuth';
import { supportedLanguages, languageLabels, type SupportedLanguage } from '@core/i18n';
import { useSaveSettings } from '../hooks/useSettings';
import {
  useRetentionSettings,
  useSaveRetentionSettings,
  usePurgeAuditLog,
  usePurgeActivityLog,
} from '../hooks/useRetentionSettings';
import type { AppSettings } from '../types';

type GeneralTabProps = {
  settings?: AppSettings;
};

type PurgeTarget = 'audit' | 'activity' | null;

export function GeneralTab({ settings }: GeneralTabProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const save = useSaveSettings();
  const updatePreferences = useAuth((s) => s.updatePreferences);
  const language = useLanguageStore((s) => s.language);
  const [appName, setAppName] = useState(settings?.appName ?? 'McHarbor');
  const [refreshInterval, setRefreshInterval] = useState(settings?.autoRefreshInterval ?? 10);

  const { data: retention } = useRetentionSettings();
  const saveRetention = useSaveRetentionSettings();
  const [auditDays, setAuditDays] = useState(90);
  const [activityDays, setActivityDays] = useState(30);

  useEffect(() => {
    if (retention) {
      setAuditDays(retention.auditRetentionDays);
      setActivityDays(retention.activityRetentionDays);
    }
  }, [retention]);

  const [purgeTarget, setPurgeTarget] = useState<PurgeTarget>(null);

  const purgeAudit = usePurgeAuditLog();
  const purgeActivity = usePurgeActivityLog();

  const auditRetentionDays = retention?.auditRetentionDays ?? 0;
  const activityRetentionDays = retention?.activityRetentionDays ?? 0;

  const auditEligible = auditRetentionDays > 0;
  const activityEligible = activityRetentionDays > 0;

  const auditClearDisabled = !auditEligible;
  const activityClearDisabled = !activityEligible;

  const auditClearTooltip = useMemo(() => {
    if (!auditEligible) return t('retention.keepForever');
    return t('retention.clearNowHint', { days: auditRetentionDays });
  }, [auditEligible, auditRetentionDays, t]);

  const activityClearTooltip = useMemo(() => {
    if (!activityEligible) return t('retention.keepForever');
    return t('retention.clearNowHint', { days: activityRetentionDays });
  }, [activityEligible, activityRetentionDays, t]);

  const activeMutationPending =
    purgeTarget === 'audit' ? purgeAudit.isPending : purgeActivity.isPending;

  const openConfirm = (target: 'audit' | 'activity') => setPurgeTarget(target);
  const closeConfirm = () => {
    if (activeMutationPending) return;
    setPurgeTarget(null);
  };

  const confirmTitle =
    purgeTarget === 'audit' ? t('retention.clearAuditTitle') : t('retention.clearActivityTitle');

  const confirmDescription = useMemo(() => {
    if (purgeTarget === 'audit') {
      return t('retention.clearAuditDescription', { days: auditRetentionDays });
    }
    if (purgeTarget === 'activity') {
      return t('retention.clearActivityDescription', { days: activityRetentionDays });
    }
    return '';
  }, [purgeTarget, auditRetentionDays, activityRetentionDays, t]);

  const handleConfirm = () => {
    if (purgeTarget === 'audit') {
      purgeAudit.mutate(undefined, { onSuccess: () => setPurgeTarget(null) });
    } else if (purgeTarget === 'activity') {
      purgeActivity.mutate(undefined, { onSuccess: () => setPurgeTarget(null) });
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <Label className="mb-2">{t('general.language')}</Label>
        <p className="mb-2 text-sm text-muted-foreground">{t('general.languageDescription')}</p>
        <Select
          value={language}
          onChange={(v) => {
            void updatePreferences({ preferredLanguage: v as SupportedLanguage });
          }}
          options={supportedLanguages.map((lang) => ({ value: lang, label: languageLabels[lang] }))}
          className="max-w-sm"
        />
      </div>
      <div>
        <Label className="mb-2">{t('general.appName')}</Label>
        <Input
          type="text"
          value={appName}
          onChange={(e) => setAppName(e.target.value)}
          className="max-w-sm"
        />
      </div>
      <div>
        <Label className="mb-2">{t('general.autoRefreshInterval')}</Label>
        <NumberInput
          value={refreshInterval}
          onChange={setRefreshInterval}
          min={1}
          max={300}
          className="w-40"
        />
      </div>
      <Button
        onClick={() => save.mutate({ appName, autoRefreshInterval: refreshInterval })}
        disabled={save.isPending}
      >
        {save.isPending ? t('general.saving') : tc('actions.save')}
      </Button>

      <hr className="border-border" />

      <div>
        <h3 className="text-sm font-semibold">{t('retention.title')}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{t('retention.description')}</p>
      </div>
      <div>
        <Label className="mb-2">{t('retention.auditDays')}</Label>
        <p className="mb-2 text-sm text-muted-foreground">{t('retention.auditDaysDescription')}</p>
        <div className="flex flex-wrap items-center gap-3">
          <NumberInput
            value={auditDays}
            onChange={setAuditDays}
            min={0}
            max={3650}
            className="w-40"
          />
          <Tooltip>
            <TooltipTrigger asChild>
              <span tabIndex={auditClearDisabled ? 0 : -1}>
                <Button
                  type="button"
                  variant="destructive"
                  size="sm"
                  onClick={() => openConfirm('audit')}
                  disabled={auditClearDisabled}
                  aria-label={t('retention.clearNow')}
                >
                  <IconEraser className="size-4" />
                  {t('retention.clearNow')}
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>{auditClearTooltip}</TooltipContent>
          </Tooltip>
        </div>
      </div>
      <div>
        <Label className="mb-2">{t('retention.activityDays')}</Label>
        <p className="mb-2 text-sm text-muted-foreground">{t('retention.activityDaysDescription')}</p>
        <div className="flex flex-wrap items-center gap-3">
          <NumberInput
            value={activityDays}
            onChange={setActivityDays}
            min={0}
            max={3650}
            className="w-40"
          />
          <Tooltip>
            <TooltipTrigger asChild>
              <span tabIndex={activityClearDisabled ? 0 : -1}>
                <Button
                  type="button"
                  variant="destructive"
                  size="sm"
                  onClick={() => openConfirm('activity')}
                  disabled={activityClearDisabled}
                  aria-label={t('retention.clearNow')}
                >
                  <IconEraser className="size-4" />
                  {t('retention.clearNow')}
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>{activityClearTooltip}</TooltipContent>
          </Tooltip>
        </div>
      </div>
      <Button
        onClick={() => saveRetention.mutate({
          auditRetentionDays: auditDays,
          activityRetentionDays: activityDays,
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
