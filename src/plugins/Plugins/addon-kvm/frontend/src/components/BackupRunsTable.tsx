// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconAlertTriangle } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { StatusError } from '@resources/components/ui/StatusError';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import type { BackupRun } from '../hooks/useVMBackups';
import { RunRowActions } from './BackupRowActions';
import { formatBytes, formatRelative } from '../format';

export function BackupRunsTable({
  runs,
  deletePending,
  onRestore,
  onDelete,
}: {
  runs: BackupRun[];
  deletePending: boolean;
  onRestore: (run: BackupRun) => void;
  onDelete: (run: BackupRun) => void;
}) {
  const { t } = useTranslation('vm');

  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('backups.columns.started')}</TableHead>
            <TableHead>{t('backups.columns.kind')}</TableHead>
            <TableHead>{t('backups.columns.size')}</TableHead>
            <TableHead>{t('backups.columns.consistency')}</TableHead>
            <TableHead>{t('backups.columns.destinations')}</TableHead>
            <TableHead className="text-right">{t('backups.columns.status')}</TableHead>
            <TableHead className="w-24 text-right">{t('labels.actions', { ns: 'common' })}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {runs.map((run) => (
            <TableRow key={run.id}>
              <TableCell>{formatRelative(run.startedAt)}</TableCell>
              <TableCell>
                <span className="flex items-center gap-1.5">
                  <Badge variant={run.kind === 'full' ? 'secondary' : 'outline'}>{t(`backups.kind.${run.kind}`)}</Badge>
                  {run.fellBackToFull && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <IconAlertTriangle className="h-4 w-4 text-warning" aria-label={t('backups.fellBack')} />
                      </TooltipTrigger>
                      <TooltipContent className="max-w-[340px]">
                        {t('backups.fellBackHint', {
                          reason: run.fallbackReason ?? '',
                        })}
                      </TooltipContent>
                    </Tooltip>
                  )}
                </span>
              </TableCell>
              <TableCell>{formatBytes(run.bytesTotal)}</TableCell>
              <TableCell>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="text-muted-foreground">{t(`backups.consistency.${run.consistency}`)}</span>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-[340px]">
                    {t(`backups.consistency.${run.consistency}Hint`)}
                  </TooltipContent>
                </Tooltip>
              </TableCell>
              <TableCell>
                <span className="flex flex-wrap gap-1">
                  {run.destinations.map((dest) => (
                    <Badge key={dest.id} variant={dest.status === 'success' ? 'success' : 'destructive'}>
                      {dest.locationName || dest.locationId}
                    </Badge>
                  ))}
                </span>
              </TableCell>
              <TableCell>
                <div className="flex items-center justify-end gap-2">
                  <Badge variant={statusVariant(run.status)}>{t(`backups.status.${run.status}`)}</Badge>
                  <StatusError error={run.error} />
                </div>
              </TableCell>
              <TableCell>
                <RunRowActions run={run} deletePending={deletePending} onRestore={onRestore} onDelete={onDelete} />
              </TableCell>
            </TableRow>
          ))}
          {runs.length === 0 && (
            <TableRow>
              <TableCell colSpan={7} className="py-6 text-center text-muted-foreground">
                {t('backups.noRuns')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}

function statusVariant(runStatus: string): 'success' | 'destructive' | 'warning' | 'secondary' | 'outline' {
  switch (runStatus) {
    case 'success':
      return 'success';
    case 'failure':
      return 'destructive';
    case 'running':
      return 'warning';
    default:
      return 'outline';
  }
}
