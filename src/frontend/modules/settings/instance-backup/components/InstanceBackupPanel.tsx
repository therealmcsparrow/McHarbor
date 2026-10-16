// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArchive, IconPlus, IconRestore } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { PERM } from '@core/auth/permissionKeys';
import { useDeleteExport, useInstanceBackups, type InstanceExport } from '../hooks/useInstanceBackups';
import { CreateExportDialog } from './CreateExportDialog';
import { DownloadExportDialog } from './DownloadExportDialog';
import { ExportHistoryTable } from './ExportHistoryTable';
import { RestoreDialog } from './RestoreDialog';
import { RestoreStatusCard } from './RestoreStatusCard';
import { VerifyExportDialog } from './VerifyExportDialog';
import { ScheduleList } from './ScheduleList';
import { useRestoreStatus } from '../hooks/useInstanceRestore';

export function InstanceBackupPanel() {
  const { t } = useTranslation('instancebackup');
  const exports = useInstanceBackups();
  const remove = useDeleteExport();
  const [creating, setCreating] = useState(false);
  const [downloading, setDownloading] = useState<InstanceExport | null>(null);
  const [deleting, setDeleting] = useState<InstanceExport | null>(null);
  const [restoring, setRestoring] = useState<string | null>(null);
  const [verifying, setVerifying] = useState<InstanceExport | null>(null);
  const restoreStatus = useRestoreStatus();
  const restoreExport = useCallback((e: InstanceExport) => setRestoring(e.id), []);

  const createButton = (
    <Button permission={PERM.INSTANCE_BACKUP_MANAGE} onClick={() => setCreating(true)}>
      <IconPlus className="size-4" />
      {t('panel.create')}
    </Button>
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex max-w-3xl flex-col gap-2">
          <p className="text-sm text-muted-foreground">{t('panel.intro')}</p>
          <p className="text-sm text-muted-foreground">{t('panel.twoBackups')}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" permission={PERM.INSTANCE_BACKUP_RESTORE} onClick={() => setRestoring('')}>
            <IconRestore className="size-4" />
            {t('restore.open')}
          </Button>
          {createButton}
        </div>
      </div>
      <Alert variant="warning">{t('panel.keyWarning')}</Alert>
      {restoreStatus.data && <RestoreStatusCard status={restoreStatus.data} />}
      <ScheduleList />

      {!exports.isLoading && (exports.data ?? []).length === 0 ? (
        <EmptyState
          icon={<IconArchive className="size-6" />}
          title={t('empty.title')}
          description={t('empty.description')}
        >
          {createButton}
        </EmptyState>
      ) : (
        <ExportHistoryTable
          exports={exports.data ?? []}
          loading={exports.isLoading}
          isFetching={exports.isFetching}
          onRefresh={() => void exports.refetch()}
          onDownload={setDownloading}
          onDelete={setDeleting}
          onRestore={restoreExport}
          onVerify={setVerifying}
        />
      )}

      <CreateExportDialog open={creating} onOpenChange={setCreating} />
      {restoring !== null && (
        <RestoreDialog
          open
          initialExportId={restoring}
          exports={exports.data ?? []}
          onOpenChange={(isOpen) => !isOpen && setRestoring(null)}
        />
      )}
      <VerifyExportDialog target={verifying} onOpenChange={(isOpen) => !isOpen && setVerifying(null)} />
      <DownloadExportDialog target={downloading} onOpenChange={(isOpen) => !isOpen && setDownloading(null)} />
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(isOpen) => {
          if (isOpen) return;
          setDeleting(null);
          remove.reset();
        }}
        title={t('delete.title')}
        description={t('delete.description', { name: deleting?.fileName ?? '' })}
        confirmLabel={t('delete.confirm')}
        variant="destructive"
        loading={remove.isPending}
        permission={PERM.INSTANCE_BACKUP_MANAGE}
        onConfirm={() => deleting && remove.mutate(deleting.id, { onSuccess: () => setDeleting(null) })}
      >
        {remove.error && <Alert variant="destructive">{remove.error.message}</Alert>}
      </ConfirmDialog>
    </div>
  );
}
