// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PlainButton } from '@resources/components/ui/PlainButton';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { IconAlertTriangle, IconCamera, IconPlayerPause, IconBolt } from '@tabler/icons-react';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { cn } from '@resources/utils/cn';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useSnapshotConfigs } from '@resources/hooks/useSnapshotConfigs';
import type { ContainerBackupInput } from '@resources/hooks/useContainerBackups';

type BackupConsistencyFieldProps = {
  value: ContainerBackupInput;
  onChange: (patch: Partial<ContainerBackupInput>) => void;
};

const MODES = ['pause', 'snapshot', 'live'] as const;
type ConsistencyMode = (typeof MODES)[number];

const MODE_ICONS = {
  pause: IconPlayerPause,
  snapshot: IconCamera,
  live: IconBolt,
} as const;

/**
 * How this backup stays consistent, and what it costs.
 *
 * This is where snapshots are switched on: a driver being installed
 * makes it possible, but a plan asking for it is what makes it happen.
 * Snapshot mode stays unselectable until the environment has an enabled
 * snapshot configuration, because a plan that asks to freeze with
 * nothing to freeze it with would quietly pause instead.
 */
export function BackupConsistencyField({ value, onChange }: BackupConsistencyFieldProps) {
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((state) => state.currentId);
  const configs = useSnapshotConfigs(envId);

  const usable = useMemo(() => (configs.data ?? []).filter((config) => config.enabled), [configs.data]);
  const mode = (value.consistencyMode ?? 'pause') as ConsistencyMode;
  const snapshotUnavailable = usable.length === 0;

  function pickMode(next: string) {
    if (next === 'snapshot') {
      onChange({
        consistencyMode: 'snapshot',
        // Preselect the only sensible answer when there is exactly one,
        // so the common case takes one click instead of two.
        snapshotConfigId: value.snapshotConfigId || usable[0]?.id || '',
      });
      return;
    }
    onChange({ consistencyMode: next, snapshotConfigId: '' });
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="backup-consistency">{t('backups.consistency.label')}</Label>
        <p className="text-xs text-muted-foreground">{t('backups.consistency.description')}</p>
      </div>

      <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
        {MODES.map((option) => {
          const Icon = MODE_ICONS[option];
          const disabled = option === 'snapshot' && snapshotUnavailable;
          const selected = mode === option;
          return (
            <PlainButton
              key={option}
              type="button"
              disabled={disabled}
              aria-pressed={selected}
              onClick={() => pickMode(option)}
              className={cn(
                'rounded-lg border p-3 text-left transition-colors',
                selected ? 'border-primary bg-primary/10' : 'border-border hover:bg-muted/40',
                disabled && 'cursor-not-allowed opacity-50 hover:bg-transparent',
              )}
            >
              <span className="flex items-center gap-2 text-sm font-medium text-foreground">
                <Icon className="size-4" />
                {t(`backups.consistency.modes.${option}`)}
              </span>
              <span className="mt-1 block text-xs text-muted-foreground">
                {t(`backups.consistency.descriptions.${option}`)}
              </span>
            </PlainButton>
          );
        })}
      </div>

      {snapshotUnavailable && (
        <p className="text-xs text-muted-foreground">
          {t('backups.consistency.noConfigs')}{' '}
          <Link to="/settings?tab=snapshots" className="text-primary underline">
            {t('backups.consistency.noConfigsLink')}
          </Link>
        </p>
      )}

      {mode === 'snapshot' && usable.length > 0 && (
        <div className="space-y-2">
          <Label htmlFor="backup-snapshot-config">{t('backups.consistency.configLabel')}</Label>
          <Select
            value={value.snapshotConfigId ?? ''}
            onChange={(next: string) => onChange({ snapshotConfigId: next })}
            ariaLabel={t('backups.consistency.configLabel')}
            placeholder={t('backups.consistency.configPlaceholder')}
            options={usable.map((config) => ({
              value: config.id,
              label: `${config.name} (${config.driver})`,
            }))}
          />
          <p className="flex items-start gap-2 text-xs text-muted-foreground">
            <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" />
            {t('backups.consistency.fallbackHint')}
          </p>
        </div>
      )}
    </div>
  );
}
