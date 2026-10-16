// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { describeCron } from '@resources/utils/schedule';
import type { BackupPlan } from '../hooks/useCTBackups';
import { PlanRowActions } from './BackupRowActions';
import { formatRelative } from '../format';

interface BackupPlansTableProps {
  plans: BackupPlan[];
  runBlocked: boolean;
  runPending: boolean;
  deletePending: boolean;
  onRun: (plan: BackupPlan) => void;
  onEdit: (plan: BackupPlan) => void;
  onDelete: (plan: BackupPlan) => void;
}

export function BackupPlansTable({
  plans,
  runBlocked,
  runPending,
  deletePending,
  onRun,
  onEdit,
  onDelete,
}: BackupPlansTableProps) {
  const { t } = useTranslation('ct');
  const { t: tc } = useTranslation('common');

  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('backups.planColumns.name')}</TableHead>
            <TableHead>{t('backups.planColumns.schedule')}</TableHead>
            <TableHead>{t('backups.planColumns.retention')}</TableHead>
            <TableHead>{t('backups.planColumns.lastRun')}</TableHead>
            <TableHead>{t('backups.planColumns.nextRun')}</TableHead>
            <TableHead className="w-28 text-right">{t('labels.actions', { ns: 'common' })}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {plans.map((plan) => (
            <TableRow key={plan.id}>
              <TableCell>
                <span className="flex items-center gap-2">
                  {plan.name}
                  {!plan.enabled && <Badge variant="outline">{t('backups.disabled')}</Badge>}
                </span>
              </TableCell>
              <TableCell>
                {plan.cron ? (
                  <span>
                    {describeCron(plan.cron, tc)}
                    <span className="ml-2 font-mono text-xs opacity-70">{plan.cron}</span>
                  </span>
                ) : (
                  <span className="text-muted-foreground">—</span>
                )}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {describeRetention(plan.retentionCount, plan.retentionDays, t)}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {plan.lastRunAt ? formatRelative(plan.lastRunAt) : '—'}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {plan.nextRunAt ? formatRelative(plan.nextRunAt) : '—'}
              </TableCell>
              <TableCell>
                <PlanRowActions
                  plan={plan}
                  runBlocked={runBlocked}
                  runPending={runPending}
                  deletePending={deletePending}
                  onRun={onRun}
                  onEdit={onEdit}
                  onDelete={onDelete}
                />
              </TableCell>
            </TableRow>
          ))}
          {plans.length === 0 && (
            <TableRow>
              <TableCell colSpan={6} className="py-6 text-center text-muted-foreground">
                {t('backups.noPlans')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}

function describeRetention(
  count: number,
  days: number,
  t: (key: string, options?: Record<string, unknown>) => string,
): string {
  const parts: string[] = [];
  if (count > 0) parts.push(t('backups.retention.lastCount', { count }));
  if (days > 0) parts.push(t('backups.retention.forDays', { days }));
  return parts.length > 0 ? parts.join(' · ') : t('backups.retention.forever');
}
