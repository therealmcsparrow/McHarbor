// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { EnvVarDef } from '../types';

interface InstallStepEnvVarsProps {
  envVarDefs: EnvVarDef[];
  envVars: Record<string, string>;
  onEnvVarChange: (key: string, value: string) => void;
}

export function InstallStepEnvVars({ envVarDefs, envVars, onEnvVarChange }: InstallStepEnvVarsProps) {
  const { t } = useTranslation('common');

  if (envVarDefs.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('appStore.noEnvVars')}</p>;
  }

  return (
    <div className="space-y-3">
      {envVarDefs.map((ev) => (
        <div key={ev.key}>
          <label className="mb-1 flex items-center gap-2 text-xs font-medium text-foreground">
            <code>{ev.key}</code>
            <span className="font-normal text-muted-foreground">{ev.description}</span>
          </label>
          <input
            type={ev.secret ? 'password' : 'text'}
            value={envVars[ev.key] ?? ''}
            onChange={(e) => onEnvVarChange(ev.key, e.target.value)}
            placeholder={ev.default || t('appStore.notSet')}
            className="w-full rounded border border-border bg-background px-2 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none"
          />
        </div>
      ))}
    </div>
  );
}

