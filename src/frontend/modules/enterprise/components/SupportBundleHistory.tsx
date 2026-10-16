// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload, IconTrash } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { StatusError } from '@resources/components/ui/StatusError';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { formatBytes, formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { type Bundle, type BundleList, useDownloadBundle } from '@resources/hooks/useSupport';
import { DeleteSupportBundleDialog } from './DeleteSupportBundleDialog';

const stateTone: Record<Bundle['status'], 'info' | 'success' | 'destructive'> = {
  running: 'info',
  ready: 'success',
  failed: 'destructive',
};

export function SupportBundleHistory({ list }: { list: BundleList }) {
  const { t } = useTranslation('support');
  const [deleting, setDeleting] = useState<string | null>(null);
  const download = useDownloadBundle();

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('bundle.historyTitle')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('bundle.historyDescription', { count: list.keep })}</p>
      </CardHeader>
      <CardContent className="p-0">
        {download.error && (
          <Alert variant="destructive" className="m-4">
            {download.error.message}
          </Alert>
        )}
        {list.items.length === 0 ? (
          <p className="p-4 text-sm text-muted-foreground">{t('bundle.empty')}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('bundle.columns.created')}</TableHead>
                <TableHead>{t('bundle.columns.createdBy')}</TableHead>
                <TableHead>{t('bundle.columns.categories')}</TableHead>
                <TableHead className="text-right">{t('bundle.columns.size')}</TableHead>
                <TableHead>{t('bundle.columns.status')}</TableHead>
                <TableHead className="text-right">{t('bundle.columns.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {list.items.map((b) => (
                <TableRow key={b.id}>
                  <TableCell className="whitespace-nowrap">{formatDate(b.createdAt)}</TableCell>
                  <TableCell>{b.createdBy}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {b.categories.map((c) => t(`bundle.category.${c}.title`)).join(', ')}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {b.status === 'ready' ? formatBytes(b.sizeBytes) : '-'}
                  </TableCell>
                  <TableCell>
                    <div className="flex min-w-[140px] items-center gap-2">
                      <Badge variant={stateTone[b.status]}>{t(`bundle.state.${b.status}`)}</Badge>
                      {b.status === 'running' && (
                        <ProgressBar value={b.progress} aria-label={t('bundle.running', { progress: b.progress })} />
                      )}
                      <StatusError error={b.status === 'failed' ? t('bundle.failed') : undefined} />
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t('bundle.download')}
                        permission={PERM.SUPPORT_MANAGE}
                        disabled={b.status !== 'ready' || (download.isPending && download.variables === b.id)}
                        onClick={() => download.mutate(b.id)}
                      >
                        <IconDownload className="size-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t('bundle.delete')}
                        permission={PERM.SUPPORT_MANAGE}
                        disabled={b.status === 'running'}
                        onClick={() => setDeleting(b.id)}
                      >
                        <IconTrash className="size-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
      <DeleteSupportBundleDialog bundleId={deleting} onOpenChange={(isOpen) => !isOpen && setDeleting(null)} />
    </Card>
  );
}
