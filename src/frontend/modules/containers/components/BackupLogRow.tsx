// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { formatDate } from '@resources/utils/format';
import { severityStyle } from '@resources/utils/severity';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import type { BackupLog } from '../hooks/useBackupLogs';

type BackupLogRowProps = {
  log: BackupLog;
};

export function BackupLogRow({ log }: BackupLogRowProps) {
  const { t } = useTranslation('containers');
  const formatPrefs = useLocaleFormat();
  const style = severityStyle(log.severity);
  return (
    <tr className="border-t border-border align-top transition-colors hover:bg-muted/30">
      <td className="px-3 py-3 font-mono text-xs text-muted-foreground whitespace-nowrap">
        {formatDate(log.createdAt, formatPrefs)}
      </td>
      <td className="px-3 py-3">
        <span
          className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium capitalize ${style.pill} ${style.text}`}
          aria-label={t(`backups.logs.severity.${log.severity}`)}
        >
          <span className={`size-1.5 rounded-full ${style.dot}`} aria-hidden="true" />
          {t(`backups.logs.severity.${log.severity}`)}
        </span>
      </td>
      <td className="px-3 py-3 text-sm">
        {t(`backups.logs.action.${log.action}`, { defaultValue: log.action })}
      </td>
      <td className="px-3 py-3 text-sm">
        {log.message || '—'}
      </td>
      <td className="px-3 py-3 text-xs">
        {log.containerName || log.containerId?.slice(0, 12) || '—'}
      </td>
      <td className="px-3 py-3 text-xs">
        {log.planName || log.planId?.slice(0, 12) || '—'}
      </td>
      <td className="px-3 py-3 text-xs font-mono text-muted-foreground">
        {log.environmentId?.slice(0, 8) || '—'}
      </td>
      <td className="px-3 py-3 text-xs font-mono text-muted-foreground">
        {log.phase || '—'}
      </td>
    </tr>
  );
}

type BackupLogPaginationProps = {
  page: number;
  totalPages: number;
  total: number;
  disabled: boolean;
  onPageChange: (page: number) => void;
  labels: {
    total: string;
    pageOf: string;
  };
};

export function BackupLogPagination({
  page,
  totalPages,
  total,
  disabled,
  onPageChange,
  labels,
}: BackupLogPaginationProps) {
  if (total <= 0) return null;
  return (
    <div className="flex items-center justify-between text-xs text-muted-foreground">
      <div>{labels.total.replace('{total}', String(total))}</div>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={page <= 1 || disabled}
          onClick={() => onPageChange(Math.max(1, page - 1))}
          aria-label="Previous page"
        >
          ‹
        </Button>
        <span>{labels.pageOf.replace('{page}', String(page)).replace('{totalPages}', String(totalPages))}</span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={page >= totalPages || disabled}
          onClick={() => onPageChange(Math.min(totalPages, page + 1))}
          aria-label="Next page"
        >
          ›
        </Button>
      </div>
    </div>
  );
}