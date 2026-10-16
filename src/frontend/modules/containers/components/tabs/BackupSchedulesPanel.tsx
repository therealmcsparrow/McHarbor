// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPencil, IconPlayerPlay, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { describeCron } from '@resources/utils/schedule';
import { formatDate } from '@resources/utils/format';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import type { ContainerBackupPlan } from '@resources/hooks/useContainerBackups';

type BackupSchedulesPanelProps = {
  plans: ContainerBackupPlan[];
  onRun: (plan: ContainerBackupPlan) => void;
  onEdit: (plan: ContainerBackupPlan) => void;
  onDelete: (plan: ContainerBackupPlan) => void;
  runPending: boolean;
  editPending: boolean;
};

function retentionSummary(plan: ContainerBackupPlan, t: (key: string, options?: Record<string, number>) => string) {
  if (plan.retentionCount > 0 && plan.retentionDays > 0) {
    return t('backups.retentionCountDays', {
      count: plan.retentionCount,
      days: plan.retentionDays,
    });
  }
  if (plan.retentionCount > 0) {
    return t('backups.retentionCountOnly', { count: plan.retentionCount });
  }
  if (plan.retentionDays > 0) {
    return t('backups.retentionDaysOnly', { count: plan.retentionDays, days: plan.retentionDays });
  }
  return t('backups.retentionForever');
}

export function BackupSchedulesPanel({
  plans,
  onRun,
  onEdit,
  onDelete,
  runPending,
  editPending,
}: BackupSchedulesPanelProps) {
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const formatPrefs = useLocaleFormat();

  return (
    <section className="mc-glass rounded-lg p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold text-foreground">{t('backups.savedSchedules')}</h2>
      </div>
      {plans.length === 0 ? (
        <p className="py-6 text-center text-sm text-muted-foreground">{t('backups.noSchedules')}</p>
      ) : (
        <div className="space-y-2">
          {plans.map((plan) => (
            <div key={plan.id} className="rounded-lg border border-border bg-background p-3">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <h3 className="truncate text-sm font-medium text-foreground">{plan.name}</h3>
                    <Badge variant={plan.enabled ? 'default' : 'secondary'}>
                      {plan.enabled ? t('backups.enabled') : t('backups.disabled')}
                    </Badge>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">{describeCron(plan.cron, tc)}</p>
                  {plan.storageLocationIds.length > 0 && (
                    <p className="text-xs text-muted-foreground">
                      {t('backups.destinationsCount', {
                        count: plan.storageLocationIds.length,
                      })}
                    </p>
                  )}
                  <p className="text-xs text-muted-foreground">{retentionSummary(plan, t)}</p>
                  {plan.nextRunAt && (
                    <p className="text-xs text-muted-foreground">
                      {t('backups.nextRun', {
                        value: formatDate(plan.nextRunAt, formatPrefs),
                      })}
                    </p>
                  )}
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('backups.runNow')}
                    permission={PERM.BACKUPS_MANAGE}
                    onClick={() => onRun(plan)}
                    disabled={runPending}
                  >
                    <IconPlayerPlay className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('backups.editSchedule')}
                    permission={PERM.BACKUPS_MANAGE}
                    onClick={() => onEdit(plan)}
                    disabled={editPending}
                  >
                    <IconPencil className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('backups.deleteSchedule')}
                    permission={PERM.BACKUPS_DELETE}
                    onClick={() => onDelete(plan)}
                  >
                    <IconTrash className="size-4 text-destructive" />
                  </Button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
