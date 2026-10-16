// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCircleCheck, IconCircleX, IconPlus } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ListItem } from '@resources/components/ui/ListItem';
import { PERM } from '@core/auth/permissionKeys';
import type { SnapshotDriverStatus } from '../hooks/useSnapshotConfigs';

/**
 * One detected driver. Extracted from the card body so the grouping stays
 * legible next to the loading, error and empty branches it sits beside.
 */
export function DriverRow({
  row,
  onSetUp,
}: {
  row: SnapshotDriverStatus;
  onSetUp: (driver: SnapshotDriverStatus) => void;
}) {
  const { t } = useTranslation('settings');
  return (
    <ListItem
      title={row.displayName || row.driver}
      badge={
        <div className="flex items-center gap-2">
          <Badge variant={row.available ? 'default' : 'secondary'}>
            {row.available ? <IconCircleCheck className="size-3" /> : <IconCircleX className="size-3" />}
            {row.available ? t('snapshots.detection.available') : t('snapshots.detection.unavailable')}
          </Badge>
          {row.available && (
            <Badge variant={row.pauseFree ? 'default' : 'secondary'}>
              {row.pauseFree ? t('snapshots.detection.pauseFree') : t('snapshots.detection.pausesBriefly')}
            </Badge>
          )}
        </div>
      }
      subtitle={driverSubtitle(row, t)}
      actions={
        <Button
          permission={PERM.BACKUPS_MANAGE}
          variant="outline"
          size="sm"
          disabled={!row.available}
          onClick={() => onSetUp(row)}
          aria-label={t('snapshots.detection.setUp')}
        >
          <IconPlus className="size-4" />
          {t('snapshots.detection.setUp')}
        </Button>
      }
    />
  );
}

/**
 * The one line under a driver's name: where it was found, what it can
 * see, and, when it is missing, why.
 */
function driverSubtitle(
  row: SnapshotDriverStatus,
  t: (key: string, options?: Record<string, unknown>) => string,
): string {
  const parts: string[] = [t(`snapshots.detection.origin.${row.origin}`)];
  if (row.version) parts.push(row.version);
  if (row.toolPath) parts.push(row.toolPath);
  if (row.available && row.targets && row.targets.length > 0) {
    parts.push(t('snapshots.detection.targetCount', { count: row.targets.length }));
  }
  if (!row.available && row.error) parts.push(row.error);
  return parts.join(' · ');
}
