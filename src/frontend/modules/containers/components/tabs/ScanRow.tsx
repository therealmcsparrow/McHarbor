// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { formatDate } from '@resources/utils/format';
import type { Scan } from '../../hooks/useScans';

const STATUS_VARIANT: Record<string, 'success' | 'default' | 'warning' | 'destructive'> = {
  completed: 'success',
  running: 'default',
  pending: 'warning',
  failed: 'destructive',
};

type ScanRowProps = {
  scan: Scan;
  selected: boolean;
  onSelect: () => void;
  onDelete: () => void;
};

export function ScanRow({ scan, selected, onSelect, onDelete }: ScanRowProps) {
  const { t } = useTranslation('containers');
  return (
    <div
      className={`flex items-center justify-between rounded-md border px-3 py-2 text-xs transition-colors ${
        selected ? 'border-primary/50 bg-primary/5' : 'border-border bg-muted/30 hover:border-border/80'
      }`}
    >
      <Button
        type="button"
        variant="ghost"
        className="h-auto flex-1 justify-start gap-3 p-0 text-left hover:bg-transparent"
        onClick={onSelect}
      >
        <Badge variant={STATUS_VARIANT[scan.status] ?? 'secondary'} className="text-[10px]">
          {t(`scan.${scan.status}`)}
        </Badge>
        <span className="font-mono text-muted-foreground">{scan.scanner}</span>
        {scan.status === 'completed' && (
          <div className="flex gap-1.5">
            {scan.criticalCount > 0 && (
              <Badge variant="destructive" className="text-[10px]">
                {t('scan.severityBadge.critical', { count: scan.criticalCount })}
              </Badge>
            )}
            {scan.highCount > 0 && (
              <Badge variant="destructive" className="text-[10px]">
                {t('scan.severityBadge.high', { count: scan.highCount })}
              </Badge>
            )}
            {scan.mediumCount > 0 && (
              <Badge variant="warning" className="text-[10px]">
                {t('scan.severityBadge.medium', { count: scan.mediumCount })}
              </Badge>
            )}
            {scan.lowCount > 0 && (
              <Badge variant="secondary" className="text-[10px]">
                {t('scan.severityBadge.low', { count: scan.lowCount })}
              </Badge>
            )}
          </div>
        )}
        <span className="ml-auto text-muted-foreground">
          {scan.completedAt ? formatDate(scan.completedAt) : formatDate(scan.startedAt)}
        </span>
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('scan.deleteScan')}
        className="ml-2 size-6"
        onClick={(e) => {
          e.stopPropagation();
          onDelete();
        }}
      >
        <IconTrash className="size-3.5" />
      </Button>
    </div>
  );
}
