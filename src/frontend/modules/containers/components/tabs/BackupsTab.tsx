// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconChevronDown,
  IconChevronRight,
  IconClock,
  IconDatabaseExport,
  IconDownload,
  IconPencil,
  IconPlayerPlay,
  IconPlayerStop,
  IconRestore,
  IconTrash,
  IconUpload,
} from "@tabler/icons-react";
import { Badge } from "@resources/components/ui/Badge";
import { Button } from "@resources/components/ui/Button";
import { ConfirmDialog } from "@resources/components/ui/ConfirmDialog";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@resources/components/ui/Dialog";
import { Input } from "@resources/components/ui/Input";
import { Spinner } from "@resources/components/ui/Spinner";
import { Switch } from "@resources/components/ui/Switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@resources/components/ui/Tooltip";
import { describeCron } from "@resources/utils/schedule";
import { formatDate } from "@resources/utils/format";
import { useLocaleFormat } from "@resources/hooks/useLocaleFormat";
import { useStorageLocations } from "../../../settings/hooks/useStorageLocations";
import { BackupSelectionFields } from "../BackupSelectionFields";
import {
  mandatoryLocalStorage,
  planToInput,
  withMandatoryStorage,
} from "../plan-utils";
import { BackupRunDestinations } from "./BackupRunDestinations";
import { RestoreProgressView, RestoreRunDialog } from "../RestoreRunDialog";
import {
  type ContainerBackupInput,
  type ContainerBackupOption,
  type ContainerBackupPlan,
  type ContainerBackupRun,
  containerBackupDownloadUrl,
  useCancelContainerBackupRun,
  useContainerBackupOptions,
  useContainerBackupPlans,
  useContainerBackupRun,
  useContainerBackupRuns,
  useCreateContainerBackupPlan,
  useDeleteContainerBackupPlan,
  useDeleteContainerBackupRun,
  useRunContainerBackup,
  useRunContainerBackupPlan,
  useRetryDestinationUpload,
  useUpdateContainerBackupPlan,
  useUploadRestoreContainerBackup,
} from "../../hooks/useContainerBackups";

type BackupsTabProps = {
  containerId: string;
  containerName: string;
};

const DEFAULT_INPUT: ContainerBackupInput = {
  name: "",
  storageLocationId: "",
  storageLocationIds: [],
  includeConfig: true,
  includeLogs: false,
  includeFilesystem: false,
  includeImage: false,
  selectedMounts: [],
  cron: "",
  enabled: true,
  retentionCount: 0,
  retentionDays: 0,
};

function defaultInput(
  options: ContainerBackupOption[],
  containerName: string,
): ContainerBackupInput {
  return {
    ...DEFAULT_INPUT,
    name: `${containerName} backup`,
    includeLogs: options.some(
      (option) => option.type === "logs" && option.default,
    ),
    includeFilesystem: options.some(
      (option) => option.type === "filesystem" && option.default,
    ),
    includeImage: options.some(
      (option) => option.type === "image" && option.default,
    ),
    selectedMounts: options
      .filter((option) => option.key.startsWith("mount:") && option.default)
      .map((option) => option.key.slice("mount:".length)),
  };
}

function formatBytes(value: number) {
  if (!value) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length - 1,
  );
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function retentionSummary(
  plan: ContainerBackupPlan,
  t: (key: string, options?: Record<string, number>) => string,
) {
  if (plan.retentionCount > 0 && plan.retentionDays > 0) {
    return t("backups.retentionCountDays", {
      count: plan.retentionCount,
      days: plan.retentionDays,
    });
  }
  if (plan.retentionCount > 0) {
    return t("backups.retentionCountOnly", { count: plan.retentionCount });
  }
  if (plan.retentionDays > 0) {
    return t("backups.retentionDaysOnly", { days: plan.retentionDays });
  }
  return t("backups.retentionForever");
}

function backupProgressDescription(
  run: ContainerBackupRun,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (run.status !== "running" || !run.progressStage) {
    return "";
  }
  if (run.progressMessage) {
    return run.progressMessage;
  }
  return t(`backups.progress.${run.progressStage}`, {
    defaultValue: run.progressMessage ?? "",
  });
}

// Human-readable mapping for restore_* progress stages emitted by the
// backend. The container_backup service writes these to the run record
// throughout the restore pipeline.
//
// Kept here so BackupsTab can still map stage codes for any code
// path that uses describeRestoreStage directly. The full restore
// dialog now lives in RestoreRunDialog.tsx and uses the same map.
//
// (Removed inline RestoreProgressView — moved to
// RestoreRunDialog.tsx so the cross-environment Backups page can
// reuse it without copy-paste.)

export function BackupsTab({ containerId, containerName }: BackupsTabProps) {
  const { t } = useTranslation("containers");
  const { t: tc } = useTranslation("common");
  const formatPrefs = useLocaleFormat();
  const { data: optionData, isLoading: optionsLoading } =
    useContainerBackupOptions(containerId);
  const { data: plans = [], isLoading: plansLoading } =
    useContainerBackupPlans(containerId);
  const { data: runs = [], isLoading: runsLoading } =
    useContainerBackupRuns(containerId);
  const { data: storageLocations = [], isLoading: storageLocationsLoading } =
    useStorageLocations();
  const runManual = useRunContainerBackup(containerId);
  const createPlan = useCreateContainerBackupPlan(containerId);
  const updatePlan = useUpdateContainerBackupPlan();
  const runPlan = useRunContainerBackupPlan();
  const deletePlan = useDeleteContainerBackupPlan();
  const deleteRun = useDeleteContainerBackupRun();
  const cancelRun = useCancelContainerBackupRun();
  const uploadRestoreBackup = useUploadRestoreContainerBackup(containerId);
  const retryDestinationUpload = useRetryDestinationUpload();
  const [manualInput, setManualInput] =
    useState<ContainerBackupInput>(DEFAULT_INPUT);
  const [scheduleInput, setScheduleInput] = useState<ContainerBackupInput>({
    ...DEFAULT_INPUT,
    cron: "0 3 * * *",
  });
  const [manualOpen, setManualOpen] = useState(true);
  const [scheduleOpen, setScheduleOpen] = useState(true);
  const [editTarget, setEditTarget] = useState<ContainerBackupPlan | null>(null);
  const [editInput, setEditInput] = useState<ContainerBackupInput>(DEFAULT_INPUT);
  const [deleteTarget, setDeleteTarget] = useState<ContainerBackupPlan | null>(
    null,
  );
  const [deleteRunTarget, setDeleteRunTarget] =
    useState<ContainerBackupRun | null>(null);
  const [cancelRunTarget, setCancelRunTarget] =
    useState<ContainerBackupRun | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<ContainerBackupRun | null>(
    null,
  );
  const [uploadRestoreOpen, setUploadRestoreOpen] = useState(false);
  const [uploadRestoreFile, setUploadRestoreFile] = useState<File | null>(null);
  const [uploadRestoreKey, setUploadRestoreKey] = useState("");
  const [uploadRestoreRunId, setUploadRestoreRunId] = useState<string | null>(
    null,
  );

  const options = useMemo(
    () => optionData?.options ?? [],
    [optionData?.options],
  );
  const requiredLocalStorage = useMemo(
    () => mandatoryLocalStorage(storageLocations),
    [storageLocations],
  );
  const requiredLocalStorageId = requiredLocalStorage?.id;
  const manualBackupRunning = useMemo(
    () =>
      runs.some(
        (run) =>
          run.operation === "backup" && run.status === "running" && !run.planId,
      ),
    [runs],
  );
  const { data: uploadRestoreRun, isLoading: uploadRestoreRunLoading } =
    useContainerBackupRun(containerId, uploadRestoreRunId ?? undefined);
  const uploadRestoreIsActive = uploadRestoreRun?.status === "running";

  useEffect(() => {
    if (options.length === 0) return;
    const next = defaultInput(options, containerName);
    setManualInput(withMandatoryStorage(next, requiredLocalStorageId));
    setScheduleInput(
      withMandatoryStorage(
        { ...next, cron: "0 3 * * *", enabled: true },
        requiredLocalStorageId,
      ),
    );
  }, [containerName, options, requiredLocalStorageId]);

  useEffect(() => {
    if (!editTarget) return;
    setEditInput(
      withMandatoryStorage(planToInput(editTarget), requiredLocalStorageId),
    );
  }, [editTarget, requiredLocalStorageId]);

  function updateEditInput(patch: Partial<ContainerBackupInput>) {
    setEditInput((current) =>
      withMandatoryStorage({ ...current, ...patch }, requiredLocalStorageId),
    );
  }

  function openEditPlan(plan: ContainerBackupPlan) {
    setEditTarget(plan);
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

  useEffect(() => {
    if (!editTarget) return;
    setEditInput(
      withMandatoryStorage(planToInput(editTarget), requiredLocalStorageId),
    );
  }, [editTarget, requiredLocalStorageId]);

  function updateManualInput(patch: Partial<ContainerBackupInput>) {
    setManualInput((current) =>
      withMandatoryStorage({ ...current, ...patch }, requiredLocalStorageId),
    );
  }

  function updateScheduleInput(patch: Partial<ContainerBackupInput>) {
    setScheduleInput((current) =>
      withMandatoryStorage({ ...current, ...patch }, requiredLocalStorageId),
    );
  }

  if (
    optionsLoading ||
    plansLoading ||
    runsLoading ||
    storageLocationsLoading
  ) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  function handleRestore(run: ContainerBackupRun) {
    setRestoreTarget(run);
  }

  function closeUploadRestoreDialog() {
    setUploadRestoreOpen(false);
    setUploadRestoreFile(null);
    setUploadRestoreKey("");
    setUploadRestoreRunId(null);
  }

  function restoreUploadedBackup() {
    if (!uploadRestoreFile) return;
    uploadRestoreBackup.mutate(
      { file: uploadRestoreFile, secretKey: uploadRestoreKey },
      {
        onSuccess: (result) => {
          // The backend creates a new restore run and immediately starts
          // restoring. Switch the dialog into the same progress view used
          // for the in-place restore so the user sees live status.
          setUploadRestoreRunId(result.runId);
        },
      },
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(360px,0.8fr)]">
      <div className="space-y-4">
        <section className="rounded-lg border border-border bg-card p-4">
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-start gap-2">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={t("backups.manualTitle")}
                aria-expanded={manualOpen}
                onClick={() => setManualOpen((open) => !open)}
                className="-ml-1 mt-0.5 shrink-0 text-muted-foreground"
              >
                {manualOpen ? (
                  <IconChevronDown className="size-4" />
                ) : (
                  <IconChevronRight className="size-4" />
                )}
              </Button>
              <div className="min-w-0">
                <h2 className="text-sm font-semibold text-foreground">
                  {t("backups.manualTitle")}
                </h2>
                <p className="text-xs text-muted-foreground">
                  {t("backups.manualDescription")}
                </p>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Button
                onClick={() => runManual.mutate(manualInput)}
                disabled={
                  runManual.isPending ||
                  manualBackupRunning ||
                  manualInput.name.trim() === ""
                }
              >
                <IconDatabaseExport className="size-4" />
                {runManual.isPending || manualBackupRunning
                  ? t("backups.running")
                  : t("backups.runNow")}
              </Button>
            </div>
          </div>
          {manualOpen && (
            <div className="mt-4">
              <BackupSelectionFields
                value={manualInput}
                options={options}
                storageLocations={storageLocations}
                mandatoryStorageLocationId={requiredLocalStorageId}
                onChange={updateManualInput}
              />
            </div>
          )}
        </section>

        <section className="rounded-lg border border-border bg-card p-4">
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-start gap-2">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={t("backups.scheduleTitle")}
                aria-expanded={scheduleOpen}
                onClick={() => setScheduleOpen((open) => !open)}
                className="-ml-1 mt-0.5 shrink-0 text-muted-foreground"
              >
                {scheduleOpen ? (
                  <IconChevronDown className="size-4" />
                ) : (
                  <IconChevronRight className="size-4" />
                )}
              </Button>
              <div className="min-w-0">
                <h2 className="text-sm font-semibold text-foreground">
                  {t("backups.scheduleTitle")}
                </h2>
                <p className="text-xs text-muted-foreground">
                  {t("backups.scheduleDescription")}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-3">
              <span className="text-xs text-muted-foreground">
                {t("backups.enabled")}
              </span>
              <Switch
                checked={scheduleInput.enabled ?? true}
                onCheckedChange={(enabled) =>
                  setScheduleInput((current) => ({ ...current, enabled }))
                }
                aria-label={t("backups.enabled")}
              />
              <Button
                onClick={() => createPlan.mutate(scheduleInput)}
                disabled={
                  createPlan.isPending ||
                  scheduleInput.name.trim() === "" ||
                  !scheduleInput.cron?.trim()
                }
              >
                <IconClock className="size-4" />
                {createPlan.isPending
                  ? t("backups.saving")
                  : t("backups.saveSchedule")}
              </Button>
            </div>
          </div>
          {scheduleOpen && (
            <div className="mt-4">
              <BackupSelectionFields
                value={scheduleInput}
                options={options}
                storageLocations={storageLocations}
                mandatoryStorageLocationId={requiredLocalStorageId}
                showCron
                onChange={updateScheduleInput}
              />
            </div>
          )}
        </section>
      </div>

      <div className="space-y-4">
        <section className="rounded-lg border border-border bg-card p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <h2 className="text-sm font-semibold text-foreground">
              {t("backups.savedSchedules")}
            </h2>
          </div>
          {plans.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {t("backups.noSchedules")}
            </p>
          ) : (
            <div className="space-y-2">
              {plans.map((plan) => (
                <div
                  key={plan.id}
                  className="rounded-lg border border-border bg-background p-3"
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <h3 className="truncate text-sm font-medium text-foreground">
                          {plan.name}
                        </h3>
                        <Badge variant={plan.enabled ? "default" : "secondary"}>
                          {plan.enabled
                            ? t("backups.enabled")
                            : t("backups.disabled")}
                        </Badge>
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {describeCron(plan.cron, tc)}
                      </p>
                      {plan.storageLocationIds.length > 0 && (
                        <p className="text-xs text-muted-foreground">
                          {t("backups.destinationsCount", {
                            count: plan.storageLocationIds.length,
                          })}
                        </p>
                      )}
                      <p className="text-xs text-muted-foreground">
                        {retentionSummary(plan, t)}
                      </p>
                      {plan.nextRunAt && (
                        <p className="text-xs text-muted-foreground">
                          {t("backups.nextRun", {
                            value: formatDate(plan.nextRunAt, formatPrefs),
                          })}
                        </p>
                      )}
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.runNow")}
                        onClick={() => runPlan.mutate(plan.id)}
                        disabled={runPlan.isPending}
                      >
                        <IconPlayerPlay className="size-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.editSchedule")}
                        onClick={() => openEditPlan(plan)}
                        disabled={updatePlan.isPending}
                      >
                        <IconPencil className="size-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.deleteSchedule")}
                        onClick={() => setDeleteTarget(plan)}
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

        <section className="rounded-lg border border-border bg-card p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <h2 className="text-sm font-semibold text-foreground">
              {t("detail.backups")}
            </h2>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setUploadRestoreOpen(true)}
            >
              <IconUpload className="size-4" />
              {t("backups.restoreFromFile")}
            </Button>
          </div>
          {runs.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {t("backups.noRuns")}
            </p>
          ) : (
            <div className="space-y-2">
              {runs.map((run) => (
                <div
                  key={run.id}
                  className="rounded-lg border border-border bg-background p-3"
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <Badge variant="outline">
                          {t(`backups.operation.${run.operation ?? "backup"}`)}
                        </Badge>
                        <Badge
                          variant={
                            run.status === "success"
                              ? "success"
                              : run.status === "failure"
                                ? "destructive"
                                : "secondary"
                          }
                        >
                          {run.status === "running" && <Spinner size="sm" />}
                          {t(`backups.status.${run.status}`)}
                        </Badge>
                        <span className="text-xs text-muted-foreground">
                          {formatDate(run.startedAt, formatPrefs)}
                        </span>
                      </div>
                      {backupProgressDescription(run, t) && (
                        <p className="mt-1 text-xs text-muted-foreground">
                          {backupProgressDescription(run, t)}
                        </p>
                      )}
                      <BackupRunDestinations
                        run={run}
                        onRestore={handleRestore}
                        onDelete={(target) => setDeleteRunTarget(target)}
                        onRetryUpload={(destination) => {
                          retryDestinationUpload.mutate({
                            runId: run.id,
                            destinationId: destination.id,
                          });
                        }}
                        retryPendingFor={retryDestinationUpload.isPending ? retryDestinationUpload.variables?.destinationId : undefined}
                      />
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <span className="text-xs text-muted-foreground">
                        {formatBytes(run.archiveSize)}
                      </span>
                      {run.operation === "backup" &&
                        run.status === "success" &&
                        run.archivePath && (
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            asChild
                            aria-label={t("backups.download")}
                          >
                            <a
                              href={containerBackupDownloadUrl(run.id)}
                              download
                            >
                              <IconDownload className="size-4" />
                            </a>
                          </Button>
                        )}
                      {run.status === "running" && (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={t("backups.cancelRun")}
                              data-testid={`cancel-run-${run.id}`}
                              onClick={() => setCancelRunTarget(run)}
                              disabled={cancelRun.isPending}
                              className="text-amber-500 hover:bg-amber-500/10 hover:border-amber-500/30"
                            >
                              <IconPlayerStop className="size-4" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t("backups.cancelRun")}</TooltipContent>
                        </Tooltip>
                      )}
                      {run.status !== "running" && (
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={t("backups.deleteRun")}
                          onClick={() => setDeleteRunTarget(run)}
                          disabled={deleteRun.isPending}
                        >
                          <IconTrash className="size-4" />
                        </Button>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>

      {deleteTarget && (
        <ConfirmDialog
          open={!!deleteTarget}
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("backups.deleteScheduleTitle")}
          description={t("backups.deleteScheduleDescription")}
          confirmLabel={t("backups.deleteSchedule")}
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
          onOpenChange={(open) => {
            if (!open && !updatePlan.isPending) closeEditPlan();
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
              <BackupSelectionFields
                value={editInput}
                options={options}
                storageLocations={storageLocations}
                mandatoryStorageLocationId={requiredLocalStorageId}
                showCron
                onChange={updateEditInput}
              />
            </DialogBody>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={closeEditPlan}
                disabled={updatePlan.isPending}
              >
                {t("backups.cancelRestore")}
              </Button>
              <Button
                onClick={saveEditPlan}
                disabled={
                  updatePlan.isPending ||
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
      )}
      {deleteRunTarget && (
        <ConfirmDialog
          open={!!deleteRunTarget}
          onOpenChange={(open) => {
            if (!open) setDeleteRunTarget(null);
          }}
          title={t("backups.deleteRunTitle")}
          description={t("backups.deleteRunDescription")}
          confirmLabel={t("backups.deleteRun")}
          loading={deleteRun.isPending}
          onConfirm={() => {
            deleteRun.mutate(deleteRunTarget.id, {
              onSuccess: () => setDeleteRunTarget(null),
            });
          }}
        />
      )}
      {cancelRunTarget && (
        <ConfirmDialog
          open={!!cancelRunTarget}
          onOpenChange={(open) => {
            if (!open) setCancelRunTarget(null);
          }}
          title={t("backups.cancelRunTitle")}
          description={t("backups.cancelRunDescription")}
          confirmLabel={t("backups.cancelRunConfirm")}
          loading={cancelRun.isPending}
          onConfirm={() => {
            cancelRun.mutate(cancelRunTarget.id, {
              onSuccess: () => setCancelRunTarget(null),
            });
          }}
        />
      )}
      <RestoreRunDialog
        run={restoreTarget}
        containerId={containerId}
        onClose={() => setRestoreTarget(null)}
      />
      <Dialog
        open={uploadRestoreOpen}
        onOpenChange={(open) => {
          if (!open && (uploadRestoreIsActive || uploadRestoreRunId === null)) {
            closeUploadRestoreDialog();
          }
          if (open) setUploadRestoreOpen(true);
        }}
      >
        <DialogContent>
          <DialogHeader>
            {uploadRestoreRunId ? (
              <DialogTitle>
                {uploadRestoreRun?.status === "failure"
                  ? t("backups.restoreFailedTitle")
                  : uploadRestoreIsActive
                    ? t("backups.restoringTitle")
                    : t("backups.restoreCompleteTitle")}
              </DialogTitle>
            ) : (
              <DialogTitle>{t("backups.restoreUploadTitle")}</DialogTitle>
            )}
            {uploadRestoreRunId ? null : (
              <DialogDescription>
                {t("backups.restoreUploadDescription")}
              </DialogDescription>
            )}
          </DialogHeader>
          <DialogBody className="space-y-4">
            {uploadRestoreRunId ? (
              <RestoreProgressView
                run={uploadRestoreRun}
                loading={uploadRestoreRunLoading}
                startedAt={new Date().toISOString()}
              />
            ) : (
              <>
                <div>
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    {t("backups.restoreUploadFile")}
                  </label>
                  <input
                    type="file"
                    accept=".tar,.mcharbor.tar,application/x-tar"
                    onChange={(event) =>
                      setUploadRestoreFile(event.target.files?.[0] ?? null)
                    }
                    className="block w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground file:mr-3 file:rounded-md file:border-0 file:bg-secondary file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-secondary-foreground hover:file:bg-secondary/80 focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    {t("backups.restoreUploadKey")}
                  </label>
                  <Input
                    type="password"
                    value={uploadRestoreKey}
                    onChange={(event) => setUploadRestoreKey(event.target.value)}
                    placeholder={t("backups.restoreUploadKeyPlaceholder")}
                    autoComplete="off"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("backups.restoreUploadKeyHint")}
                  </p>
                </div>
                <p className="text-xs text-muted-foreground">
                  {t("backups.restoreWarning")}
                </p>
              </>
            )}
          </DialogBody>
          <DialogFooter>
            {uploadRestoreRunId ? (
              <>
                {uploadRestoreIsActive && (
                  <Button
                    variant="destructive"
                    onClick={() => {
                      if (uploadRestoreRun) {
                        cancelRun.mutate(uploadRestoreRun.id);
                      }
                    }}
                    disabled={cancelRun.isPending}
                    data-testid="upload-restore-cancel"
                  >
                    <IconPlayerStop className="size-4" />
                    {t("backups.cancelRunConfirm")}
                  </Button>
                )}
                <Button
                  onClick={closeUploadRestoreDialog}
                  disabled={uploadRestoreIsActive}
                  data-testid="upload-restore-close"
                >
                  {uploadRestoreIsActive
                    ? t("backups.restoring")
                    : t("backups.closeRestore")}
                </Button>
              </>
            ) : (
              <>
                <Button
                  variant="outline"
                  onClick={closeUploadRestoreDialog}
                  disabled={uploadRestoreBackup.isPending}
                >
                  {t("backups.cancelRestore")}
                </Button>
                <Button
                  variant="destructive"
                  onClick={restoreUploadedBackup}
                  disabled={
                    uploadRestoreBackup.isPending || !uploadRestoreFile
                  }
                >
                  <IconRestore className="size-4" />
                  {uploadRestoreBackup.isPending
                    ? t("backups.restoring")
                    : t("backups.restore")}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
