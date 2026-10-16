// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPencil, IconPlayerPlay, IconRestore, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { VM_PERM } from '../permissions';
import type { BackupPlan, BackupRun } from '../hooks/useVMBackups';

interface ActionProps {
  label: string;
  permission: string;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}

function Action({ label, permission, disabled = false, onClick, children }: ActionProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span>
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
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

interface PlanRowActionsProps {
  plan: BackupPlan;
  runBlocked: boolean;
  runPending: boolean;
  deletePending: boolean;
  onRun: (plan: BackupPlan) => void;
  onEdit: (plan: BackupPlan) => void;
  onDelete: (plan: BackupPlan) => void;
}

export function PlanRowActions({
  plan,
  runBlocked,
  runPending,
  deletePending,
  onRun,
  onEdit,
  onDelete,
}: PlanRowActionsProps) {
  const { t } = useTranslation('vm');
  const [confirming, setConfirming] = useState(false);

  return (
    <div className="flex items-center justify-end gap-1">
      <Action
        label={t('backups.runNow')}
        permission={VM_PERM.BACKUP_MANAGE}
        disabled={runBlocked || runPending}
        onClick={() => onRun(plan)}
      >
        <IconPlayerPlay className="h-4 w-4" aria-hidden="true" />
      </Action>
      <Action label={t('backups.editPlan')} permission={VM_PERM.BACKUP_MANAGE} onClick={() => onEdit(plan)}>
        <IconPencil className="h-4 w-4" aria-hidden="true" />
      </Action>
      <Action
        label={t('backups.deletePlan')}
        permission={VM_PERM.BACKUP_DELETE}
        disabled={deletePending}
        onClick={() => setConfirming(true)}
      >
        <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
      </Action>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('backups.deletePlanConfirm.title')}
        description={t('backups.deletePlanConfirm.body')}
        confirmLabel={t('backups.deletePlan')}
        loading={deletePending}
        onConfirm={() => {
          onDelete(plan);
          setConfirming(false);
        }}
      />
    </div>
  );
}

interface RunRowActionsProps {
  run: BackupRun;
  deletePending: boolean;
  onRestore: (run: BackupRun) => void;
  onDelete: (run: BackupRun) => void;
}

export function RunRowActions({ run, deletePending, onRestore, onDelete }: RunRowActionsProps) {
  const { t } = useTranslation('vm');
  const [confirming, setConfirming] = useState(false);
  const canRestore = run.status === 'success' && run.operation === 'backup';

  return (
    <div className="flex items-center justify-end gap-1">
      <Action
        label={t('backups.restore')}
        permission={VM_PERM.RESTORE_RUN}
        disabled={!canRestore}
        onClick={() => onRestore(run)}
      >
        <IconRestore className="h-4 w-4" aria-hidden="true" />
      </Action>
      <Action
        label={t('backups.delete')}
        permission={VM_PERM.BACKUP_DELETE}
        disabled={deletePending || run.status === 'running'}
        onClick={() => setConfirming(true)}
      >
        <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
      </Action>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('backups.deleteConfirm.title')}
        description={t('backups.deleteConfirm.body')}
        confirmLabel={t('backups.delete')}
        loading={deletePending}
        onConfirm={() => {
          onDelete(run);
          setConfirming(false);
        }}
      />
    </div>
  );
}
