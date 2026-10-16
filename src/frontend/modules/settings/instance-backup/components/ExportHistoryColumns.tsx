// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload, IconRestore, IconShieldCheck, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { formatBytes, formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import type { DestinationStatus, ExportStatus, InstanceExport } from '../hooks/useInstanceBackups';

export type ExportRowActions = {
  onDownload: (e: InstanceExport) => void;
  onDelete: (e: InstanceExport) => void;
  onRestore: (e: InstanceExport) => void;
  onVerify: (e: InstanceExport) => void;
};

const STATUS_VARIANT: Record<ExportStatus, 'success' | 'destructive' | 'info'> = {
  completed: 'success',
  failed: 'destructive',
  running: 'info',
};

function StatusCell({ e }: { e: InstanceExport }) {
  const { t } = useTranslation('instancebackup');
  return (
    <div className="flex items-center gap-2">
      <Badge variant={STATUS_VARIANT[e.status]}>{t(`status.${e.status}`)}</Badge>
      <StatusError error={e.error} title={t('errorTitle')} />
      {e.status === 'running' && e.progress && e.progress.total > 0 && (
        <span className="text-xs text-muted-foreground">
          {t('progress', { done: e.progress.done, total: e.progress.total })}
        </span>
      )}
    </div>
  );
}

const DESTINATION_VARIANT: Record<DestinationStatus, 'success' | 'destructive' | 'info'> = {
  success: 'success',
  failure: 'destructive',
  uploading: 'info',
};

function CopiesCell({ e }: { e: InstanceExport }) {
  const { t } = useTranslation('instancebackup');
  const destinations = e.destinations ?? [];
  if (destinations.length === 0) {
    return <span className="text-muted-foreground">{t('destinations.hostOnly')}</span>;
  }
  return (
    <div className="flex flex-col gap-1">
      {destinations.map((d) => (
        <div key={d.id} className="flex items-center gap-2">
          <Badge variant={DESTINATION_VARIANT[d.status]}>{t(`destinations.${d.status}`)}</Badge>
          <span className="truncate">{d.locationName}</span>
          <StatusError error={d.error} title={t('destinations.errorTitle')} />
        </div>
      ))}
    </div>
  );
}

function VerifyCell({ e }: { e: InstanceExport }) {
  const { t } = useTranslation('instancebackup');
  if (e.verifyStatus === '') return <span className="text-muted-foreground">-</span>;
  if (e.verifyStatus === 'running') return <Badge variant="info">{t('verify.running')}</Badge>;
  return (
    <div className="flex items-center gap-2">
      <Badge variant={e.verifyStatus === 'passed' ? 'success' : 'destructive'}>{t(`verify.${e.verifyStatus}`)}</Badge>
      <StatusError error={e.verifyError} title={t('verify.errorTitle')} />
      {e.verifiedAt && <span className="text-xs text-muted-foreground">{formatDate(e.verifiedAt)}</span>}
    </div>
  );
}

function ActionsCell({ e, actions }: { e: InstanceExport; actions: ExportRowActions }) {
  const { t } = useTranslation('instancebackup');
  const completed = e.status === 'completed';
  return (
    <div className="flex justify-end gap-1">
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('actions.download')}
        permission={PERM.INSTANCE_BACKUP_MANAGE}
        disabled={!completed}
        onClick={() => actions.onDownload(e)}
      >
        <IconDownload className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('actions.verify')}
        permission={PERM.INSTANCE_BACKUP_MANAGE}
        disabled={!completed || e.verifyStatus === 'running'}
        onClick={() => actions.onVerify(e)}
      >
        <IconShieldCheck className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('actions.restore')}
        permission={PERM.INSTANCE_BACKUP_RESTORE}
        disabled={!completed}
        onClick={() => actions.onRestore(e)}
      >
        <IconRestore className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('actions.delete')}
        permission={PERM.INSTANCE_BACKUP_MANAGE}
        onClick={() => actions.onDelete(e)}
      >
        <IconTrash className="size-4" />
      </Button>
    </div>
  );
}

export function useExportHistoryColumns(actions: ExportRowActions): ColumnDef<InstanceExport, unknown>[] {
  const { t } = useTranslation('instancebackup');
  const { onDownload, onDelete, onRestore, onVerify } = actions;

  return useMemo<ColumnDef<InstanceExport, unknown>[]>(
    () => [
      {
        id: 'created',
        accessorFn: (e) => e.createdAt,
        header: t('table.created'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => <span className="whitespace-nowrap">{formatDate(row.original.createdAt)}</span>,
      },
      {
        id: 'status',
        accessorFn: (e) => t(`status.${e.status}`),
        header: t('table.status'),
        cell: ({ row }) => <StatusCell e={row.original} />,
      },
      {
        id: 'trigger',
        accessorFn: (e) => (e.trigger === 'manual' || e.trigger === 'schedule' ? t(`trigger.${e.trigger}`) : e.trigger),
        header: t('table.trigger'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
      },
      {
        id: 'size',
        accessorFn: (e) => e.sizeBytes,
        header: t('table.size'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (row.original.status === 'completed' ? formatBytes(row.original.sizeBytes) : '-'),
      },
      {
        id: 'key',
        accessorFn: (e) => [t(`keyKind.${e.keyKind}`), e.keyId].filter(Boolean).join(' '),
        header: t('table.key'),
        cell: ({ row }) => (
          <>
            {t(`keyKind.${row.original.keyKind}`)}
            {row.original.keyId && (
              <span className="ml-2 font-mono text-xs text-muted-foreground">{row.original.keyId}</span>
            )}
          </>
        ),
      },
      {
        id: 'verified',
        accessorFn: (e) => (e.verifyStatus ? t(`verify.${e.verifyStatus}`) : ''),
        header: t('table.verified'),
        cell: ({ row }) => <VerifyCell e={row.original} />,
      },
      {
        id: 'copies',
        accessorFn: (e) =>
          (e.destinations ?? []).map((d) => `${d.locationName} ${t(`destinations.${d.status}`)}`).join(' '),
        header: t('table.copies'),
        enableSorting: false,
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => <CopiesCell e={row.original} />,
      },
      {
        id: 'createdBy',
        accessorFn: (e) => e.createdBy,
        header: t('table.createdBy'),
      },
      {
        id: 'note',
        accessorFn: (e) => e.note,
        header: t('table.note'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => <span className="block max-w-[16rem] break-words">{row.original.note}</span>,
      },
      {
        id: 'fileName',
        accessorFn: (e) => e.fileName,
        header: t('table.fileName'),
        meta: { defaultVisible: false } satisfies TableColumnMeta,
        cell: ({ row }) => <span className="font-mono text-xs">{row.original.fileName}</span>,
      },
      {
        id: 'actions',
        header: t('table.actions'),
        enableSorting: false,
        meta: { width: 'lg', locked: true, noExport: true, label: t('table.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => <ActionsCell e={row.original} actions={{ onDownload, onDelete, onRestore, onVerify }} />,
      },
    ],
    [t, onDownload, onDelete, onRestore, onVerify],
  );
}
