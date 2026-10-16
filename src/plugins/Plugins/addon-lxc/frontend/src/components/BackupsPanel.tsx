// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { CT_PERM } from '../permissions';
import {
  useBackupPlans,
  useBackupRuns,
  useDeleteBackupPlan,
  useDeleteBackupRun,
  useRunBackup,
  type BackupPlan,
  type BackupRun,
} from '../hooks/useCTBackups';
import { useStorageLocations } from '../hooks/useStorageLocations';
import { BackupPlanDialog } from './BackupPlanDialog';
import { BackupRestoreDialog } from './BackupRestoreDialog';
import { RecreateFromBackupDialog } from './RecreateFromBackupDialog';
import { BackupPlansTable } from './BackupPlansTable';
import { BackupRunsTable } from './BackupRunsTable';
import type { Container } from '../types';

export function BackupsPanel({ container }: { container: Container }) {
  const { t } = useTranslation('ct');

  const plans = useBackupPlans(container.id);
  const runs = useBackupRuns(container.id);
  const runBackup = useRunBackup(container.id);
  const deletePlan = useDeleteBackupPlan(container.id);
  const deleteRun = useDeleteBackupRun(container.id);
  const locations = useStorageLocations();

  const [editing, setEditing] = useState<BackupPlan | null>(null);
  const [creating, setCreating] = useState(false);
  const [restoring, setRestoring] = useState<BackupRun | null>(null);
  const [recreating, setRecreating] = useState<BackupRun | null>(null);

  const planList = plans.data ?? [];
  const runList = runs.data ?? [];
  const hasPlan = planList.length > 0;
  const hasDestination = (locations.data ?? []).length > 0;

  const blocker = container.isTemplate
    ? t('backups.blocked.template')
    : !container.hostAvailable
      ? t('backups.blocked.hostOffline')
      : !hasDestination
        ? t('backups.blocked.noDestination')
        : !hasPlan
          ? t('backups.blocked.noPlan')
          : '';

  return (
    <div className="space-y-8">
      {blocker && (
        <p className="flex items-start gap-2 rounded-md bg-warning/10 p-3 text-sm text-foreground">
          <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
          {blocker}
        </p>
      )}

      <section>
        <div className="mb-2 flex items-start justify-between gap-2">
          <div>
            <h3 className="font-medium text-foreground">{t('backups.title')}</h3>
            <p className="text-sm text-muted-foreground">{t('backups.description')}</p>
          </div>
          <div className="flex gap-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    variant="outline"
                    size="sm"
                    permission={CT_PERM.BACKUP}
                    disabled={Boolean(blocker) || runBackup.isPending}
                    onClick={() => runBackup.mutate({ planId: planList[0]?.id })}
                  >
                    {t('backups.runNow')}
                  </Button>
                </span>
              </TooltipTrigger>
              {blocker && <TooltipContent className="max-w-[340px]">{blocker}</TooltipContent>}
            </Tooltip>
            <Button
              size="sm"
              permission={CT_PERM.BACKUP}
              disabled={container.isTemplate}
              onClick={() => setCreating(true)}
            >
              <IconPlus className="mr-1.5 h-4 w-4" aria-hidden="true" />
              {t('backups.addPlan')}
            </Button>
          </div>
        </div>

        <BackupPlansTable
          plans={planList}
          runBlocked={Boolean(blocker)}
          runPending={runBackup.isPending}
          deletePending={deletePlan.isPending}
          onRun={(entry) => runBackup.mutate({ planId: entry.id })}
          onEdit={setEditing}
          onDelete={(entry) => deletePlan.mutate(entry.id)}
        />
      </section>

      <section>
        <h3 className="mb-2 font-medium text-foreground">{t('backups.history')}</h3>
        <BackupRunsTable
          runs={runList}
          deletePending={deleteRun.isPending}
          onRestore={setRestoring}
          onRecreate={setRecreating}
          onDelete={(entry) => deleteRun.mutate(entry.id)}
        />
      </section>

      {(creating || editing) && (
        <BackupPlanDialog
          container={container}
          plan={editing ?? undefined}
          onOpenChange={(next) => {
            if (next) return;
            setCreating(false);
            setEditing(null);
          }}
        />
      )}
      {recreating && (
        <RecreateFromBackupDialog
          run={recreating}
          onOpenChange={(next) => {
            if (!next) setRecreating(null);
          }}
        />
      )}
      {restoring && (
        <BackupRestoreDialog
          container={container}
          run={restoring}
          onOpenChange={(next) => {
            if (!next) setRestoring(null);
          }}
        />
      )}
    </div>
  );
}
