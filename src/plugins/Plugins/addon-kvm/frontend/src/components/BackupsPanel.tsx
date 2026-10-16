// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { VM_PERM } from '../permissions';
import {
  useBackupPlans,
  useBackupRuns,
  useDeleteBackupPlan,
  useDeleteBackupRun,
  useRunBackup,
  type BackupRun,
} from '../hooks/useVMBackups';
import { useBackupKeyStatus } from '../hooks/useBackupKey';
import { useStorageLocations } from '../hooks/useStorageLocations';
import { BackupPlansTable } from './BackupPlansTable';
import { BackupRunsTable } from './BackupRunsTable';
import { BackupPlanDialog } from './BackupPlanDialog';
import { BackupRestoreDialog } from './BackupRestoreDialog';
import type { VM } from '../types';

/**
 * A machine's backup schedules and history.
 *
 * History first in the layout: an operator opening this tab almost always
 * wants to know whether last night's backup worked, not to edit the schedule.
 */
export type BackupIntent = { kind: 'restore' | 'plan'; id: string };

export function BackupsPanel({
  vm,
  intent,
  onIntentHandled,
}: {
  vm: VM;
  intent?: BackupIntent;
  onIntentHandled?: () => void;
}) {
  const { t } = useTranslation('vm');

  const plans = useBackupPlans(vm.id);
  const runs = useBackupRuns(vm.id);
  const runBackup = useRunBackup(vm.id);
  const deletePlan = useDeleteBackupPlan(vm.id);
  const deleteRun = useDeleteBackupRun(vm.id);

  const [editing, setEditing] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [restoring, setRestoring] = useState<BackupRun | null>(null);

  const intentKey = intent ? `${intent.kind}:${intent.id}` : '';
  const [handledKey, setHandledKey] = useState('');
  if (intent && handledKey !== intentKey) {
    if (intent.kind === 'restore') {
      if (runs.data) {
        const run = runs.data.find((entry) => entry.id === intent.id);
        if (run) setRestoring(run);
        setHandledKey(intentKey);
      }
    } else if (plans.data) {
      if (plans.data.some((entry) => entry.id === intent.id)) setEditing(intent.id);
      setHandledKey(intentKey);
    }
  }

  const notifiedKey = useRef('');
  useEffect(() => {
    if (!intentKey || handledKey !== intentKey || notifiedKey.current === intentKey) return;
    notifiedKey.current = intentKey;
    onIntentHandled?.();
  }, [intentKey, handledKey, onIntentHandled]);

  const keyStatus = useBackupKeyStatus();
  const locations = useStorageLocations();

  const hasPlan = (plans.data ?? []).length > 0;
  const hasDestination = (locations.data ?? []).length > 0;
  // Only treated as missing when the answer actually came back. A refusal
  // means this operator cannot read settings, not that the key is absent.
  const keyMissing = keyStatus.isSuccess && !keyStatus.data?.configured;

  // Said in the order they have to be fixed, so the notice names the next
  // step rather than every step.
  const blocker = keyMissing
    ? t('backups.blocked.noKey')
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
                    permission={VM_PERM.BACKUP_MANAGE}
                    disabled={Boolean(blocker) || runBackup.isPending}
                    onClick={() => runBackup.mutate({ planId: plans.data?.[0]?.id })}
                  >
                    {t('backups.runNow')}
                  </Button>
                </span>
              </TooltipTrigger>
              {blocker && <TooltipContent className="max-w-[340px]">{blocker}</TooltipContent>}
            </Tooltip>
            <Button size="sm" permission={VM_PERM.BACKUP_MANAGE} onClick={() => setCreating(true)}>
              <IconPlus className="mr-1.5 h-4 w-4" aria-hidden="true" />
              {t('backups.addPlan')}
            </Button>
          </div>
        </div>

        <BackupPlansTable
          plans={plans.data ?? []}
          blocked={Boolean(blocker)}
          runPending={runBackup.isPending}
          deletePending={deletePlan.isPending}
          onRun={(entry) => runBackup.mutate({ planId: entry.id })}
          onEdit={(entry) => setEditing(entry.id)}
          onDelete={(entry) => deletePlan.mutate(entry.id)}
        />
      </section>

      <section>
        <h3 className="mb-2 font-medium text-foreground">{t('backups.tabs.runs')}</h3>
        <BackupRunsTable
          runs={runs.data ?? []}
          deletePending={deleteRun.isPending}
          onRestore={setRestoring}
          onDelete={(entry) => deleteRun.mutate(entry.id)}
        />
      </section>

      {(creating || editing) && (
        <BackupPlanDialog
          vm={vm}
          plan={(plans.data ?? []).find((entry) => entry.id === editing)}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
        />
      )}
      {restoring && <BackupRestoreDialog vm={vm} run={restoring} onClose={() => setRestoring(null)} />}
    </div>
  );
}
