// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { useStorageLocations } from '@resources/hooks/useStorageLocations';
import { useContainer } from '../../hooks/useContainers';
import { BackupSelectionFields, isFullBackup } from '../BackupSelectionFields';
import { hasDestination, planToInput, withPrimaryDestination } from '../plan-utils';
import { BackupSchedulesPanel } from './BackupSchedulesPanel';
import {
  type ContainerBackupInput,
  type ContainerBackupPlan,
  useContainerBackupOptions,
  useContainerBackupPlans,
  useDeleteContainerBackupPlan,
  useRunContainerBackupPlan,
  useUpdateContainerBackupPlan,
} from '@resources/hooks/useContainerBackups';

type BackupSchedulesTabProps = {
  containerId: string;
};

const EMPTY_INPUT: ContainerBackupInput = {
  name: '',
  storageLocationId: '',
  storageLocationIds: [],
  includeConfig: true,
  includeLogs: false,
  includeFilesystem: false,
  includeImage: false,
  selectedMounts: [],
  cron: '',
  enabled: true,
  retentionCount: 0,
  retentionDays: 0,
};

export function BackupSchedulesTab({ containerId }: BackupSchedulesTabProps) {
  const { t } = useTranslation('containers');
  const { data: optionData, isLoading: optionsLoading } = useContainerBackupOptions(containerId);
  const { data: plans = [], isLoading: plansLoading } = useContainerBackupPlans(containerId);
  const { data: storageLocations = [], isLoading: storageLocationsLoading } = useStorageLocations();
  const { data: containerInspect } = useContainer(containerId);
  const updatePlan = useUpdateContainerBackupPlan();
  const runPlan = useRunContainerBackupPlan();
  const deletePlan = useDeleteContainerBackupPlan();

  const [editTarget, setEditTarget] = useState<ContainerBackupPlan | null>(null);
  const [editInput, setEditInput] = useState<ContainerBackupInput>(EMPTY_INPUT);
  const [editFullBackup, setEditFullBackup] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ContainerBackupPlan | null>(null);

  const options = useMemo(() => optionData?.options ?? [], [optionData?.options]);
  const configSections = useMemo(() => optionData?.configSections ?? [], [optionData?.configSections]);
  const [seeded, setSeeded] = useState<{ target: ContainerBackupPlan | null; options: typeof options }>({
    target: null,
    options,
  });
  if (editTarget && (seeded.target !== editTarget || seeded.options !== options)) {
    const next = withPrimaryDestination(planToInput(editTarget));
    setSeeded({ target: editTarget, options });
    setEditInput(next);
    setEditFullBackup(isFullBackup(next, options));
  }

  function updateEditInput(patch: Partial<ContainerBackupInput>) {
    setEditInput((current) => withPrimaryDestination({ ...current, ...patch }));
  }

  function closeEditPlan() {
    setEditTarget(null);
  }

  function saveEditPlan() {
    if (!editTarget) return;
    updatePlan.mutate(
      { id: editTarget.id, body: editInput },
      {
        onSuccess: () => setEditTarget(null),
      },
    );
  }

  if (optionsLoading || plansLoading || storageLocationsLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <BackupSchedulesPanel
        plans={plans}
        onRun={(plan) => runPlan.mutate(plan.id)}
        onEdit={(plan) => setEditTarget(plan)}
        onDelete={(plan) => setDeleteTarget(plan)}
        runPending={runPlan.isPending}
        editPending={updatePlan.isPending}
      />

      {deleteTarget && (
        <ConfirmDialog
          open={!!deleteTarget}
          onOpenChange={(isOpen) => {
            if (!isOpen) setDeleteTarget(null);
          }}
          title={t('backups.deleteScheduleTitle')}
          description={t('backups.deleteScheduleDescription')}
          confirmLabel={t('backups.deleteSchedule')}
          loading={deletePlan.isPending}
          onConfirm={() => {
            deletePlan.mutate(deleteTarget.id, {
              onSuccess: () => setDeleteTarget(null),
            });
          }}
        />
      )}
      {editTarget && (
        <Dialog
          open={!!editTarget}
          onOpenChange={(isOpen) => {
            if (!isOpen && !updatePlan.isPending) closeEditPlan();
          }}
        >
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>{t('backups.editScheduleTitle')}</DialogTitle>
              <DialogDescription>{t('backups.editScheduleDescription')}</DialogDescription>
            </DialogHeader>
            <DialogBody className="space-y-4">
              <BackupSelectionFields
                value={editInput}
                options={options}
                storageLocations={storageLocations}
                containerId={containerId}
                configSections={configSections}
                containerInspect={containerInspect as Record<string, unknown> | undefined}
                showCron
                fullBackup={editFullBackup}
                onFullBackupChange={setEditFullBackup}
                onChange={updateEditInput}
              />
            </DialogBody>
            <DialogFooter>
              <Button variant="outline" onClick={closeEditPlan} disabled={updatePlan.isPending}>
                {t('backups.cancelRestore')}
              </Button>
              <Button
                onClick={saveEditPlan}
                disabled={
                  updatePlan.isPending ||
                  editInput.name.trim() === '' ||
                  !editInput.cron?.trim() ||
                  !hasDestination(editInput)
                }
              >
                {updatePlan.isPending ? t('backups.saving') : t('backups.saveChanges')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
