// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { KeyValueEditor } from '@resources/components/KeyValueEditor';
import { SecretPicker } from '@resources/components/secrets/SecretPicker';
import { SecretFileModeToggle } from '@resources/components/secrets/SecretFileModeToggle';
import type { EnvVarDef } from '../types';

interface InstallStepEnvVarsProps {
  secretFileMode: boolean;
  onSecretFileModeChange: (value: boolean) => void;
  envVarDefs: EnvVarDef[];
  envEntries: Array<{ key: string; value: string }>;
  timezone?: string;
  onEnvEntriesChange: (entries: Array<{ key: string; value: string }>) => void;
}

export function InstallStepEnvVars({
  envVarDefs,
  envEntries,
  timezone,
  onEnvEntriesChange,
  secretFileMode,
  onSecretFileModeChange,
}: InstallStepEnvVarsProps) {
  const { t } = useTranslation('common');
  const describedEnvVars = envVarDefs.filter((ev) => ev.description);

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-medium text-foreground">{t('appStore.envVars')}</h3>
        <p className="mt-1 text-xs text-muted-foreground">
          {t('appStore.environmentVariablesDescription', { timezone: timezone || 'UTC' })}
        </p>
      </div>

      <KeyValueEditor
        entries={envEntries}
        onChange={onEnvEntriesChange}
        keyLabel={t('appStore.envKey')}
        valueLabel={t('appStore.envValue')}
        addLabel={t('appStore.addEnvVar')}
      />

      {/*
        A credential this app needs can come from the vault instead of being
        typed here. The picker adds a ${secret:NAME} reference row; the value
        is substituted server-side at deploy time and never reaches the
        browser.
      */}
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground">{t('picker.label', { ns: 'secrets' })}</span>
        <SecretPicker
          className="w-72"
          value=""
          onChange={(reference) => onEnvEntriesChange([...envEntries, { key: '', value: reference }])}
        />
      </div>

      <SecretFileModeToggle checked={secretFileMode} onCheckedChange={onSecretFileModeChange} />

      {describedEnvVars.length > 0 && (
        <div className="rounded-md border border-border bg-muted/30 p-3">
          <h4 className="text-xs font-medium text-muted-foreground">{t('appStore.catalogEnvVars')}</h4>
          <div className="mt-2 space-y-1.5">
            {describedEnvVars.map((ev) => (
              <div key={ev.key} className="grid gap-1 text-xs sm:grid-cols-[minmax(0,0.35fr)_1fr]">
                <code className="flex items-center gap-1.5 font-mono text-foreground">
                  {ev.key}
                  {/*
                    The catalog marks this variable as a credential. Saying so
                    here is what turns the picker above from a feature nobody
                    notices into the obvious thing to do for this field.
                  */}
                  {ev.secret && <Badge variant="warning">{t('badges.credential', { ns: 'secrets' })}</Badge>}
                </code>
                <span className="text-muted-foreground">
                  {ev.description}
                  {ev.secret ? ` — ${t('picker.label', { ns: 'secrets' })}` : ''}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
