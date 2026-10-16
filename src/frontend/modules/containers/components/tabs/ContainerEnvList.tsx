// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconEyeOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { isRedactedValue } from '@resources/utils/env-redaction';
import { useContainerEnvRevealed } from '../../hooks/useContainerEnvRevealed';

type EnvEntry = { key: string; value: string };

type ContainerEnvListProps = {
  containerId: string;
  entries: EnvEntry[];
};

function toEntries(env: string[]): EnvEntry[] {
  return env.map((entry) => {
    const idx = entry.indexOf('=');
    return idx > -1 ? { key: entry.slice(0, idx), value: entry.slice(idx + 1) } : { key: entry, value: '' };
  });
}

export function ContainerEnvList({ containerId, entries }: ContainerEnvListProps) {
  const { t } = useTranslation('containers');
  const [revealed, setRevealed] = useState(false);
  const hasRedacted = entries.some((entry) => isRedactedValue(entry.value));
  const revealQuery = useContainerEnvRevealed(containerId, revealed);

  // A value McHarbor never reveals (its own keys) still arrives as the marker here.
  const shown = useMemo(
    () => (revealed && revealQuery.data ? toEntries(revealQuery.data) : entries),
    [entries, revealQuery.data, revealed],
  );

  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('overview.noEnvVars')}</p>;
  }

  return (
    <div>
      {(hasRedacted || revealed) && (
        <div className="mb-3 flex items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">{t('environment.valuesWithheld')}</p>
          <Button
            variant="ghost"
            size="sm"
            permission={PERM.CONTAINERS_MANAGE}
            disabled={revealed && revealQuery.isFetching}
            onClick={() => setRevealed((value) => !value)}
          >
            {revealed ? <IconEyeOff className="h-3.5 w-3.5" /> : <IconEye className="h-3.5 w-3.5" />}
            {revealed ? t('environment.hideValues') : t('environment.revealValues')}
          </Button>
        </div>
      )}
      <div className="max-h-64 overflow-y-auto">
        {shown.map((entry) => (
          <div key={`${entry.key}:${entry.value}`} className="flex gap-2 border-b border-border py-1.5 last:border-0">
            <span className="shrink-0 font-mono text-xs font-medium text-foreground">{entry.key}</span>
            <span className="truncate font-mono text-xs text-muted-foreground">{entry.value}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
