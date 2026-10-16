// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconArchive, IconCircleCheck, IconLock, IconAlertTriangle } from '@tabler/icons-react';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useInstanceBackups, type InstanceExport } from '@modules/settings/instance-backup/hooks/useInstanceBackups';
import { useExportSchedules } from '@modules/settings/instance-backup/hooks/useExportSchedules';
import { timeAgo } from '@resources/utils/format';
import {
  IconTile,
  ROW_HEIGHT,
  WidgetBadge,
  WidgetEmpty,
  WidgetFooterLink,
  WidgetFrame,
  WidgetList,
  WidgetRow,
  useFittedRows,
} from '@modules/dashboard/components/widget-kit';

export const STALE_HOURS = 48;

export function freshness(exports: InstanceExport[], now = Date.now()) {
  const completed = exports.find((e) => e.status === 'completed');
  const verified = exports.find((e) => e.verifyStatus === 'passed' && e.verifiedAt);
  const stale = !completed || now - Date.parse(completed.completedAt ?? completed.createdAt) > STALE_HOURS * 3_600_000;
  return { completed, verified, stale };
}

export default function InstanceBackupStatusWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const allowed = usePermission(PERM.INSTANCE_BACKUP_VIEW);
  const exports = useInstanceBackups(allowed);
  const schedules = useExportSchedules(allowed);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT, 2);
  const { completed, verified, stale } = freshness(exports.data ?? []);
  const failing = (schedules.data ?? []).filter((s) => s.lastStatus === 'failed');
  const ordered = [...(schedules.data ?? [])].sort(
    (a, b) => Number(b.lastStatus === 'failed') - Number(a.lastStatus === 'failed'),
  );
  const shown = ordered.slice(0, Math.max(capacity - 1, 1));
  const openSettings = () => navigate('/settings?tab=instance-backup');

  return (
    <WidgetFrame
      title={t('instanceBackupWidget.title')}
      badge={failing.length > 0 ? <WidgetBadge tone="danger">{failing.length}</WidgetBadge> : undefined}
      footer={allowed ? <WidgetFooterLink label={t('instanceBackupWidget.open')} onClick={openSettings} /> : undefined}
    >
      {!allowed ? (
        <WidgetEmpty
          icon={<IconLock className="size-5" aria-hidden="true" />}
          title={t('instanceBackupWidget.noAccess')}
        />
      ) : exports.isError ? (
        <WidgetEmpty
          icon={<IconArchive className="size-5" aria-hidden="true" />}
          title={t('instanceBackupWidget.unavailable')}
        />
      ) : (
        <WidgetList listRef={ref}>
          <li>
            <WidgetRow
              leading={
                <IconTile tone={stale ? 'warning' : 'success'}>
                  {stale ? (
                    <IconAlertTriangle className="size-4" aria-hidden="true" />
                  ) : (
                    <IconCircleCheck className="size-4" aria-hidden="true" />
                  )}
                </IconTile>
              }
              title={
                completed
                  ? t('instanceBackupWidget.lastExport', {
                      when: timeAgo(completed.completedAt ?? completed.createdAt),
                    })
                  : t('instanceBackupWidget.never')
              }
              subtitle={
                verified?.verifiedAt
                  ? t('instanceBackupWidget.lastVerified', { when: timeAgo(verified.verifiedAt) })
                  : t('instanceBackupWidget.neverVerified')
              }
              onClick={openSettings}
            />
          </li>
          {shown.map((s) => (
            <li key={s.id}>
              <WidgetRow
                leading={
                  <IconTile tone={s.lastStatus === 'failed' ? 'danger' : 'neutral'}>
                    <IconArchive className="size-4" aria-hidden="true" />
                  </IconTile>
                }
                title={s.name}
                subtitle={
                  s.lastStatus === 'failed'
                    ? t('instanceBackupWidget.failed')
                    : s.lastRunAt
                      ? t('instanceBackupWidget.ran', { when: timeAgo(s.lastRunAt) })
                      : t('instanceBackupWidget.notRun')
                }
                onClick={openSettings}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
