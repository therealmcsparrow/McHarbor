// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import { useLanguageStore } from '@resources/stores/language';
import { useAuth } from '@core/auth/useAuth';
import { supportedLanguages, languageLabels, type SupportedLanguage } from '@core/i18n';
import { useSaveSettings } from '../hooks/useSettings';
import { useRetentionSettings, useSaveRetentionSettings } from '../hooks/useRetentionSettings';
import type { AppSettings } from '../types';

type GeneralTabProps = {
  settings?: AppSettings;
};

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
        <NumberInput
          value={auditDays}
          onChange={setAuditDays}
          min={0}
          max={3650}
          className="w-40"
        />
      </div>
      <div>
        <Label className="mb-2">{t('retention.activityDays')}</Label>
        <p className="mb-2 text-sm text-muted-foreground">{t('retention.activityDaysDescription')}</p>
        <NumberInput
          value={activityDays}
          onChange={setActivityDays}
          min={0}
          max={3650}
          className="w-40"
        />
      </div>
      <Button
        onClick={() => saveRetention.mutate({ auditRetentionDays: auditDays, activityRetentionDays: activityDays })}
        disabled={saveRetention.isPending}
      >
        {saveRetention.isPending ? t('general.saving') : tc('actions.save')}
      </Button>
    </div>
  );
}
