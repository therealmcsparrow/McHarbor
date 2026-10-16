// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPencil, IconPlus, IconRefresh, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader } from '@resources/components/ui/Card';
import { Spinner } from '@resources/components/ui/Spinner';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { ListItem } from '@resources/components/ui/ListItem';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import type { SnapshotConfig } from '../hooks/useSnapshotConfigs';

type SnapshotConfigsCardProps = {
  noEnv: boolean;
  configs: {
    data?: SnapshotConfig[];
    isLoading: boolean;
    isError: boolean;
    error: unknown;
  };
  probingId: string | null;
  onAdd: () => void;
  onProbe: (config: SnapshotConfig) => void;
  onEdit: (config: SnapshotConfig) => void;
  onDelete: (config: SnapshotConfig) => void;
};

/** The configured snapshot drivers, with per-row probe / edit / delete. */
export function SnapshotConfigsCard({
  noEnv,
  configs,
  probingId,
  onAdd,
  onProbe,
  onEdit,
  onDelete,
}: SnapshotConfigsCardProps) {
  const { t } = useTranslation('settings');
  const rows = configs.data ?? [];

  return (
    <Card>
      <CardHeader className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold">{t('snapshots.configs.title')}</h3>
          <p className="text-sm text-muted-foreground">{t('snapshots.configs.subtitle')}</p>
        </div>
        <Button permission={PERM.BACKUPS_MANAGE} onClick={onAdd} disabled={noEnv}>
          <IconPlus className="size-4" />
          {t('snapshots.configs.add')}
        </Button>
      </CardHeader>
      <CardContent>
        {noEnv && <EmptyState title={t('snapshots.configs.noEnvironments')} />}
        {!noEnv && configs.isLoading && (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        )}
        {!noEnv && configs.isError && (
          <ErrorState error={configs.error as Error} title={t('snapshots.configs.loadingFailed')} />
        )}
        {!noEnv && !configs.isLoading && rows.length === 0 && (
          <EmptyState title={t('snapshots.configs.empty')} description={t('snapshots.configs.emptyHint')} />
        )}
        {!noEnv && rows.length > 0 && (
          <div className="divide-y divide-border">
            {rows.map((cfg) => (
              <ListItem
                key={cfg.id}
                title={cfg.name}
                badge={
                  <Badge variant={cfg.enabled ? 'default' : 'secondary'}>
                    {cfg.enabled ? t('snapshots.configs.enabledLabel') : t('snapshots.configs.disabledLabel')}
                  </Badge>
                }
                subtitle={
                  `${cfg.driver} · ${cfg.pluginSlug}` +
                  (cfg.poolName ? ` · ${cfg.poolName}` : '') +
                  (cfg.lastProbedAt
                    ? ` · ${t('snapshots.configs.lastProbedAt', { date: formatDate(cfg.lastProbedAt) })}`
                    : ` · ${t('snapshots.configs.neverProbed')}`)
                }
                actions={
                  <div className="flex gap-2">
                    <Button
                      permission={PERM.BACKUPS_MANAGE}
                      variant="ghost"
                      size="sm"
                      onClick={() => onProbe(cfg)}
                      disabled={probingId === cfg.id}
                      aria-label={t('snapshots.configs.testConnection')}
                    >
                      {probingId === cfg.id ? <Spinner className="size-4" /> : <IconRefresh className="size-4" />}
                      {t('snapshots.configs.testConnection')}
                    </Button>
                    <Button
                      permission={PERM.BACKUPS_MANAGE}
                      variant="ghost"
                      size="icon"
                      onClick={() => onEdit(cfg)}
                      aria-label={t('snapshots.configs.edit')}
                    >
                      <IconPencil className="size-4" />
                    </Button>
                    <Button
                      permission={PERM.BACKUPS_MANAGE}
                      variant="ghost"
                      size="icon"
                      onClick={() => onDelete(cfg)}
                      aria-label={t('snapshots.configs.delete')}
                    >
                      <IconTrash className="size-4" />
                    </Button>
                  </div>
                }
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
