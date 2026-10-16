// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { IconExternalLink, IconPencil, IconRestore, IconTrash } from '@tabler/icons-react';
import { api } from '@core/api/client';
import { apiClientPath, fillPath, usePluginCapabilities } from '@core/plugins/capabilities';
import type { PluginCapability } from '@core/plugins/loader';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { BackupRunRow } from './backup-run-rows';
import type { BackupPlanRow } from './backup-plan-rows';

type BackupSourceRow = { sourceKind: string; sourceSlug?: string };

/**
 * The `backup-row-actions` capability of the plugin that owns this row. The row's
 * slug must be the declaring plugin, so one plugin can never act on another's rows.
 */
function useBackupRowActions(row: BackupSourceRow): PluginCapability | undefined {
  return usePluginCapabilities('backup-row-actions').find(
    (c) => c.sourceKind === row.sourceKind && c.plugin === row.sourceSlug,
  );
}

function useDeletePluginBackup(kind: 'run' | 'plan', route: string) {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');
  return useMutation({
    mutationFn: (id: string) => api.del(apiClientPath(fillPath(route, { id }))).then(assertSuccess),
    meta: {
      success: () => t(kind === 'run' ? 'backups.toast.runDeleted' : 'backups.toast.scheduleDeleted'),
    },
    onSettled: () =>
      queryClient.invalidateQueries({
        queryKey: [kind === 'run' ? 'backup-provider-runs' : 'backup-provider-plans'],
      }),
  });
}

type IconActionProps = {
  label: string;
  permission: string;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
};

function IconAction({ label, permission, disabled, onClick, children }: IconActionProps) {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={label}
      permission={permission}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}

type Declared = {
  pages: { open: string; restore: string; editPlan: string };
  routes: { deleteRun: string; deletePlan: string };
  permissions: { view: string; manage: string; delete: string; restore: string };
};

function RunActions({ run, actions }: { run: BackupRunRow; actions: Declared }) {
  const { t } = useTranslation('containers');
  const navigate = useNavigate();
  const remove = useDeletePluginBackup('run', actions.routes.deleteRun);
  const [confirming, setConfirming] = useState(false);
  const canRestore = run.operation === 'backup' && run.status === 'success';
  const page = (template: string) => fillPath(template, { subject: run.containerId, id: run.id });

  return (
    <div className="flex items-center justify-end gap-1">
      <IconAction
        label={t('backups.runs.table.restore')}
        permission={actions.permissions.restore}
        disabled={!canRestore}
        onClick={() => navigate(page(actions.pages.restore))}
      >
        <IconRestore className="size-4" aria-hidden="true" />
      </IconAction>
      <IconAction
        label={t('backups.runs.table.delete')}
        permission={actions.permissions.delete}
        disabled={remove.isPending || run.status === 'running'}
        onClick={() => setConfirming(true)}
      >
        <IconTrash className="size-4 text-destructive" aria-hidden="true" />
      </IconAction>
      <IconAction
        label={t('backups.runs.table.open')}
        permission={actions.permissions.view}
        onClick={() => navigate(page(actions.pages.open))}
      >
        <IconExternalLink className="size-4" aria-hidden="true" />
      </IconAction>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('backups.deleteRunTitle')}
        description={t('backups.deleteRunDescription')}
        confirmLabel={t('backups.deleteRun')}
        loading={remove.isPending}
        onConfirm={() => remove.mutate(run.id, { onSettled: () => setConfirming(false) })}
      />
    </div>
  );
}

function PlanActions({ plan, actions }: { plan: BackupPlanRow; actions: Declared }) {
  const { t } = useTranslation('containers');
  const navigate = useNavigate();
  const remove = useDeletePluginBackup('plan', actions.routes.deletePlan);
  const [confirming, setConfirming] = useState(false);
  const page = (template: string) => fillPath(template, { subject: plan.containerId, id: plan.id });

  return (
    <div className="flex items-center justify-end gap-1">
      <IconAction
        label={t('backups.plans.table.edit')}
        permission={actions.permissions.manage}
        onClick={() => navigate(page(actions.pages.editPlan))}
      >
        <IconPencil className="size-4" aria-hidden="true" />
      </IconAction>
      <IconAction
        label={t('backups.plans.table.delete')}
        permission={actions.permissions.delete}
        disabled={remove.isPending}
        onClick={() => setConfirming(true)}
      >
        <IconTrash className="size-4 text-destructive" aria-hidden="true" />
      </IconAction>
      <IconAction
        label={t('backups.plans.table.open')}
        permission={actions.permissions.view}
        onClick={() => navigate(page(actions.pages.open))}
      >
        <IconExternalLink className="size-4" aria-hidden="true" />
      </IconAction>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('backups.deleteScheduleTitle')}
        description={t('backups.deleteScheduleDescription')}
        confirmLabel={t('backups.deleteSchedule')}
        loading={remove.isPending}
        onConfirm={() => remove.mutate(plan.id, { onSettled: () => setConfirming(false) })}
      />
    </div>
  );
}

function declared(cap: PluginCapability | undefined): Declared | undefined {
  const pages = cap?.pages ?? {};
  const routes = cap?.routes ?? {};
  const perms = cap?.permissions ?? {};
  const { open: openPage, restore, edit_plan: editPlan } = pages;
  const { delete_run: deleteRun, delete_plan: deletePlan } = routes;
  const { view, manage, delete: remove, restore: restoreKey } = perms;
  if (!openPage || !restore || !editPlan || !deleteRun || !deletePlan) return undefined;
  if (!view || !manage || !remove || !restoreKey) return undefined;
  return {
    pages: { open: openPage, restore, editPlan },
    routes: { deleteRun, deletePlan },
    permissions: { view, manage, delete: remove, restore: restoreKey },
  };
}

/** Row actions for a run a plugin owns; renders nothing when no loaded plugin claims it. */
export function PluginRunActionsCell({ run }: { run: BackupRunRow }) {
  const actions = declared(useBackupRowActions(run));
  return actions ? <RunActions run={run} actions={actions} /> : null;
}

/** Row actions for a schedule a plugin owns; renders nothing when no loaded plugin claims it. */
export function PluginPlanActionsCell({ plan }: { plan: BackupPlanRow }) {
  const actions = declared(useBackupRowActions(plan));
  return actions ? <PlanActions plan={plan} actions={actions} /> : null;
}
