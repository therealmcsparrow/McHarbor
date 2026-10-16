// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTarget, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { formatBytes, formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { releaseLabel, type AgentRelease } from '../hooks/useAgentReleases';
import { STATUS_KEY, STATUS_VARIANT, platforms } from './agent-release-display';

const DASH = '—';
const BINARY = 'binary';
const IMAGE = 'image';

type AgentReleaseColumnsArgs = {
  settingTarget: boolean;
  onSetTarget: (release: AgentRelease) => void;
  onDelete: (release: AgentRelease) => void;
};

export function useAgentReleaseColumns({
  settingTarget,
  onSetTarget,
  onDelete,
}: AgentReleaseColumnsArgs): ColumnDef<AgentRelease, unknown>[] {
  const { t } = useTranslation('settings');

  return useMemo<ColumnDef<AgentRelease, unknown>[]>(
    () => [
      {
        id: 'version',
        accessorFn: (r) => r.version,
        header: t('agentReleases.columns.version'),
        cell: ({ row }) => (
          <span className="flex items-center gap-2">
            <span className="font-mono font-medium text-foreground">{row.original.version}</span>
            {row.original.isTarget ? <Badge variant="default">{t('agentReleases.target')}</Badge> : null}
            {row.original.inUse ? <Badge variant="outline">{t('agentReleases.inUse')}</Badge> : null}
          </span>
        ),
      },
      {
        id: 'build',
        accessorFn: (r) => r.build || DASH,
        header: t('agentReleases.columns.build'),
        cell: ({ row }) => (
          <span className="flex items-center gap-2">
            <span className="font-mono text-muted-foreground">{row.original.build || DASH}</span>
            {row.original.pinned && <Badge variant="outline">{t('agentReleases.pinned')}</Badge>}
          </span>
        ),
      },
      {
        id: 'status',
        accessorFn: (r) => t(STATUS_KEY[r.status]),
        header: t('agentReleases.columns.status'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Badge variant={STATUS_VARIANT[row.original.status]}>
              {row.original.status === 'pulling' ? <Spinner size="sm" /> : null}
              {t(STATUS_KEY[row.original.status])}
            </Badge>
            <StatusError error={row.original.lastError} title={t('agentReleases.errorTitle')} />
          </div>
        ),
      },
      {
        id: 'signature',
        accessorFn: (r) => (r.signatureStatus === 'verified' ? r.signatureKey : r.signatureStatus || DASH),
        header: t('agentReleases.columns.signature'),
        cell: ({ row }) =>
          row.original.signatureStatus === 'verified' ? (
            <span className="font-mono text-xs text-muted-foreground">
              {t('agentReleases.signedBy', { key: row.original.signatureKey })}
            </span>
          ) : (
            <span className="text-muted-foreground">{DASH}</span>
          ),
      },
      {
        id: 'platforms',
        accessorFn: (r) => platforms(r, BINARY) || DASH,
        header: t('agentReleases.columns.platforms'),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{platforms(row.original, BINARY) || DASH}</span>
        ),
      },
      {
        id: 'image',
        accessorFn: (r) => platforms(r, IMAGE) || DASH,
        header: t('agentReleases.columns.image'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-muted-foreground">{platforms(row.original, IMAGE) || DASH}</span>
            <StatusError error={row.original.imageError} title={t('agentReleases.imageErrorTitle')} />
          </div>
        ),
      },
      {
        id: 'size',
        accessorFn: (r) => r.sizeBytes,
        header: t('agentReleases.columns.size'),
        meta: { width: 'sm', defaultVisible: false } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {row.original.sizeBytes ? formatBytes(row.original.sizeBytes) : DASH}
          </span>
        ),
      },
      {
        id: 'source',
        accessorFn: (r) => r.sourceUrl || DASH,
        header: t('agentReleases.columns.source'),
        meta: { defaultVisible: false } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.sourceUrl || DASH}</span>
        ),
      },
      {
        id: 'pulled',
        accessorFn: (r) => r.pulledAt || DASH,
        header: t('agentReleases.columns.pulled'),
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.pulledAt ? formatDate(row.original.pulledAt) : DASH}
            {row.original.pulledBy ? ` · ${row.original.pulledBy}` : ''}
          </span>
        ),
      },
      {
        id: 'actions',
        header: t('labels.actions', { ns: 'common' }),
        enableSorting: false,
        meta: {
          width: 'sm',
          locked: true,
          noExport: true,
          label: t('labels.actions', { ns: 'common' }),
        } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <Button
              variant="ghost"
              size="icon-sm"
              permission={PERM.UPDATES_MANAGE}
              aria-label={t('agentReleases.setTarget', {
                version: releaseLabel(row.original),
              })}
              disabled={row.original.status !== 'ready' || row.original.isTarget || settingTarget}
              onClick={() => onSetTarget(row.original)}
            >
              <IconTarget className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              permission={PERM.UPDATES_MANAGE}
              aria-label={t('agentReleases.delete', {
                version: releaseLabel(row.original),
              })}
              disabled={row.original.status === 'pulling' || row.original.isTarget || row.original.inUse}
              onClick={() => onDelete(row.original)}
            >
              <IconTrash className="size-4" />
            </Button>
          </div>
        ),
      },
    ],
    [onDelete, onSetTarget, settingTarget, t],
  );
}
