// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@resources/components/ui/Button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@resources/components/ui/Dialog";
import { useStorageLocations } from "../../settings/hooks/useStorageLocations";
import { BackupSelectionFields } from "./BackupSelectionFields";
import {
  EMPTY_BACKUP_INPUT,
  mandatoryLocalStorage,
  planToInput,
  withMandatoryStorage,
} from "./plan-utils";
import {
  useContainerBackupOptions,
  useUpdateContainerBackupPlan,
  type ContainerBackupInput,
  type ContainerBackupPlan,
} from "../hooks/useContainerBackups";

type EditPlanDialogProps = {
  plan: ContainerBackupPlan | null;
  onClose: () => void;
};

// EditPlanDialog is a self-contained edit dialog used by both the
// per-container Backups tab and the Backups overview page. It
// resolves the plan's container options / storage locations /
// mandatory local storage on its own so callers don't need to
// pre-load anything.
export function EditPlanDialog({ plan, onClose }: EditPlanDialogProps) {
  const { t } = useTranslation("containers");
  const { data: optionData, isLoading: optionsLoading } =
    useContainerBackupOptions(plan?.containerId ?? "");
  const { data: storageLocations = [], isLoading: storageLocationsLoading } =
    useStorageLocations();
  const updatePlan = useUpdateContainerBackupPlan();
  const [editInput, setEditInput] = useState<ContainerBackupInput>(
    () => (plan ? planToInput(plan) : EMPTY_BACKUP_INPUT),
  );

  useEffect(() => {
    if (plan) {
      setEditInput(planToInput(plan));
    }
  }, [plan]);

  const options = useMemo(() => optionData?.options ?? [], [optionData?.options]);
  const requiredLocalStorageId = useMemo(
    () => mandatoryLocalStorage(storageLocations)?.id,
    [storageLocations],
  );

  function update(patch: Partial<ContainerBackupInput>) {
    setEditInput((current) =>
      withMandatoryStorage({ ...current, ...patch }, requiredLocalStorageId),
    );
  }

  const loading = optionsLoading || storageLocationsLoading;

  function save() {
    if (!plan) return;
    updatePlan.mutate(
      { id: plan.id, body: editInput },
      { onSuccess: () => onClose() },
    );
  }

  return (
    <Dialog
      open={!!plan}
      onOpenChange={(open) => {
        if (!open && !updatePlan.isPending) onClose();
      }}
    >
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("backups.editScheduleTitle")}</DialogTitle>
          <DialogDescription>
            {t("backups.editScheduleDescription")}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {loading ? (
            <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
              {t("common.loading")}
            </div>
          ) : (
            <BackupSelectionFields
              value={editInput}
              options={options}
              storageLocations={storageLocations}
              mandatoryStorageLocationId={requiredLocalStorageId}
              showCron
              onChange={update}
            />
          )}
        </DialogBody>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={onClose}
            disabled={updatePlan.isPending}
          >
            {t("backups.cancelRestore")}
          </Button>
          <Button
            onClick={save}
            disabled={
              updatePlan.isPending ||
              loading ||
              editInput.name.trim() === "" ||
              !editInput.cron?.trim()
            }
          >
            {updatePlan.isPending
              ? t("backups.saving")
              : t("backups.saveChanges")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}