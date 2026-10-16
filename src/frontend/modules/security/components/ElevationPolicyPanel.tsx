// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Badge } from '@resources/components/ui/Badge';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { PERM } from '@core/auth/permissionKeys';
import { useElevatable, useSetElevatable, type ElevatableEntry } from '@resources/hooks/useElevations';

function matches(entry: ElevatableEntry, needle: string) {
  if (!needle) return true;
  const hay = `${entry.description} ${entry.key} ${entry.origin}`.toLowerCase();
  return needle
    .toLowerCase()
    .split(/\s+/)
    .every((part) => hay.includes(part));
}

export function ElevationPolicyPanel() {
  const { t } = useTranslation('security');
  const catalog = useElevatable();
  const setElevatable = useSetElevatable();
  const gate = usePermissionGate(PERM.USERS_MANAGE);
  const [filter, setFilter] = useState('');

  const rows = useMemo(() => (catalog.data ?? []).filter((e) => matches(e, filter)), [catalog.data, filter]);
  const requestable = (catalog.data ?? []).filter((e) => e.elevatable).length;

  if (catalog.isLoading) {
    return (
      <div className="flex justify-center py-6">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder={t('elevations.policyFilter')}
          aria-label={t('elevations.policyFilter')}
          className="max-w-sm"
        />
        <span className="text-xs text-muted-foreground">
          {t('elevations.policyCount', { requestable, total: catalog.data?.length ?? 0 })}
        </span>
      </div>
      <ul className="max-h-[420px] divide-y divide-border overflow-y-auto rounded-lg border border-border">
        {rows.map((entry) => (
          <li key={entry.key} className="flex items-center justify-between gap-3 px-3 py-2">
            <div className="min-w-0">
              <p className="truncate text-sm text-foreground">{entry.description || entry.key}</p>
              <p className="truncate font-mono text-xs text-muted-foreground">
                {entry.key}
                {entry.origin !== 'host' && ` · ${entry.origin}`}
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Badge variant={entry.source === 'admin' ? 'secondary' : 'outline'}>
                {entry.source === 'admin' ? t('elevations.sourceAdmin') : t('elevations.sourceDefault')}
              </Badge>
              <Switch
                checked={entry.elevatable}
                disabled={!gate.allowed || setElevatable.isPending}
                title={gate.allowed ? undefined : gate.reason}
                aria-label={t('elevations.policyToggle', { permission: entry.description || entry.key })}
                onCheckedChange={(elevatable) => setElevatable.mutate({ key: entry.key, elevatable })}
              />
            </div>
          </li>
        ))}
        {rows.length === 0 && (
          <li className="px-3 py-6 text-center text-sm text-muted-foreground">{t('elevations.policyEmpty')}</li>
        )}
      </ul>
    </div>
  );
}
