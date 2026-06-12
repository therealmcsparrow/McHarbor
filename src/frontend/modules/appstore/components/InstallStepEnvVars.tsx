// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { KeyValueEditor } from '@resources/components/KeyValueEditor';
import type { EnvVarDef } from '../types';

interface InstallStepEnvVarsProps {
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

      {describedEnvVars.length > 0 && (
        <div className="rounded-md border border-border bg-muted/30 p-3">
          <h4 className="text-xs font-medium text-muted-foreground">{t('appStore.catalogEnvVars')}</h4>
          <div className="mt-2 space-y-1.5">
            {describedEnvVars.map((ev) => (
              <div key={ev.key} className="grid gap-1 text-xs sm:grid-cols-[minmax(0,0.35fr)_1fr]">
                <code className="font-mono text-foreground">{ev.key}</code>
                <span className="text-muted-foreground">{ev.description}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

