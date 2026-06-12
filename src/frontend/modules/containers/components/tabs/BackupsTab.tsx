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
  IconPlayerPlay,
  IconRestore,
  IconTrash,
  IconUpload,
} from "@tabler/icons-react";
import { Badge } from "@resources/components/ui/Badge";
import { Button } from "@resources/components/ui/Button";
import { Checkbox } from "@resources/components/ui/Checkbox";
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
import { describeCron } from "@resources/utils/schedule";
import { useStorageLocations } from "../../../settings/hooks/useStorageLocations";
import { BackupSelectionFields } from "../BackupSelectionFields";
import {
  type ContainerBackupInput,
  type ContainerBackupOption,
  type ContainerBackupPlan,
  type ContainerBackupRun,
  type ContainerBackupRunDestination,
  containerBackupDownloadUrl,
  useContainerBackupOptions,
  useContainerBackupPlans,
  useContainerBackupRuns,
  useCreateContainerBackupPlan,
  useDeleteContainerBackupPlan,
  useDeleteContainerBackupRun,
  useContainerBackupRestoreOptions,
  useRestoreContainerBackup,
  useRunContainerBackup,
  useRunContainerBackupPlan,
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

function mandatoryLocalStorage(
  locations: { id: string; locationType: string; basePath?: string }[],
) {
  return (
    locations.find((location) => location.id === "default-local-backup") ??
    locations.find(
      (location) =>
        location.locationType === "local" &&
        location.basePath === "/mnt/backup",
    ) ??
    locations.find((location) => location.locationType === "local") ??
    null
  );
}

function withMandatoryStorage(
  input: ContainerBackupInput,
  storageLocationId?: string,
): ContainerBackupInput {
  if (!storageLocationId) {
    return input;
  }
  const ids = new Set(input.storageLocationIds ?? []);
  ids.add(storageLocationId);
  const storageLocationIds = Array.from(ids);
  return {
    ...input,
    storageLocationId: input.storageLocationId || storageLocationId,
    storageLocationIds,
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

function destinationLabel(
  destination: ContainerBackupRunDestination,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (destination.locationType === "local") {
    if (destination.storageLocationId || destination.storageLocationName) {
      return destination.storageLocationName || t("backups.storage.external");
    }
    return t("backups.storage.local");
  }
  return destination.storageLocationName || t("backups.storage.external");
}

function BackupRunDestinations({
  run,
  t,
}: {
  run: ContainerBackupRun;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const destinations = run.destinations ?? [];

  if (destinations.length === 0) {
    return null;
  }

  return (
    <div className="mt-2 space-y-1">
      {destinations.map((destination) => (
        <div key={destination.id} className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-xs font-medium text-muted-foreground">
              {destinationLabel(destination, t)}
            </span>
            <Badge
              variant={
                destination.status === "success"
                  ? "success"
                  : destination.status === "failure"
                    ? "destructive"
                    : "secondary"
              }
            >
              {t(`backups.storageStatus.${destination.status}`)}
            </Badge>
          </div>
          {destination.path && (
            <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
              {destination.path}
            </p>
          )}
          {destination.error && (
            <p className="mt-0.5 text-xs text-destructive">
              {destination.error}
            </p>
          )}
        </div>
      ))}
    </div>
  );
}

export function BackupsTab({ containerId, containerName }: BackupsTabProps) {
  const { t } = useTranslation("containers");
  const { t: tc } = useTranslation("common");
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
  const runPlan = useRunContainerBackupPlan();
  const deletePlan = useDeleteContainerBackupPlan();
  const deleteRun = useDeleteContainerBackupRun();
  const restoreBackup = useRestoreContainerBackup(containerId);
  const uploadRestoreBackup = useUploadRestoreContainerBackup(containerId);
  const [manualInput, setManualInput] =
    useState<ContainerBackupInput>(DEFAULT_INPUT);
  const [scheduleInput, setScheduleInput] = useState<ContainerBackupInput>({
    ...DEFAULT_INPUT,
    cron: "0 3 * * *",
  });
  const [manualOpen, setManualOpen] = useState(true);
  const [scheduleOpen, setScheduleOpen] = useState(true);
  const [deleteTarget, setDeleteTarget] = useState<ContainerBackupPlan | null>(
    null,
  );
  const [deleteRunTarget, setDeleteRunTarget] =
    useState<ContainerBackupRun | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<ContainerBackupRun | null>(
    null,
  );
  const [restoreSecretKey, setRestoreSecretKey] = useState("");
  const [restoreSelectedItems, setRestoreSelectedItems] = useState<string[]>(
    [],
  );
  const [uploadRestoreOpen, setUploadRestoreOpen] = useState(false);
  const [uploadRestoreFile, setUploadRestoreFile] = useState<File | null>(null);
  const [uploadRestoreKey, setUploadRestoreKey] = useState("");

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
  const restoreOptionsEnabled =
    !!restoreTarget &&
    (!restoreTarget.requiresSecretKey || restoreSecretKey.trim() !== "");
  const {
    data: restoreOptions,
    isFetching: restoreOptionsLoading,
    isError: restoreOptionsError,
  } = useContainerBackupRestoreOptions(
    restoreTarget?.id,
    restoreSecretKey,
    restoreOptionsEnabled,
  );

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
    if (!restoreOptions) return;
    setRestoreSelectedItems(
      restoreOptions.items
        .filter((item) => item.default || item.required)
        .map((item) => item.key),
    );
  }, [restoreOptions]);

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
    setRestoreSecretKey("");
    setRestoreSelectedItems([]);
    setRestoreTarget(run);
  }

  function restoreWithCurrentKey() {
    if (!restoreTarget) return;
    restoreBackup.mutate(
      {
        id: restoreTarget.id,
        secretKey: restoreSecretKey,
        restoreItems: restoreSelectedItems,
      },
      {
        onSuccess: () => {
          setRestoreTarget(null);
          setRestoreSecretKey("");
          setRestoreSelectedItems([]);
        },
      },
    );
  }

  function closeUploadRestoreDialog() {
    setUploadRestoreOpen(false);
    setUploadRestoreFile(null);
    setUploadRestoreKey("");
  }

  function toggleRestoreItem(key: string, checked: boolean) {
    setRestoreSelectedItems((current) => {
      if (checked) {
        return current.includes(key) ? current : [...current, key];
      }
      return current.filter((item) => item !== key);
    });
  }

  function restoreUploadedBackup() {
    if (!uploadRestoreFile) return;
    uploadRestoreBackup.mutate(
      { file: uploadRestoreFile, secretKey: uploadRestoreKey },
      { onSuccess: closeUploadRestoreDialog },
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
                            value: new Date(plan.nextRunAt).toLocaleString(),
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
                          {new Date(run.startedAt).toLocaleString()}
                        </span>
                      </div>
                      {backupProgressDescription(run, t) && (
                        <p className="mt-1 text-xs text-muted-foreground">
                          {backupProgressDescription(run, t)}
                        </p>
                      )}
                      <BackupRunDestinations run={run} t={t} />
                      {run.error && (
                        <p className="mt-1 text-xs text-destructive">
                          {run.error}
                        </p>
                      )}
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <span className="text-xs text-muted-foreground">
                        {formatBytes(run.archiveSize)}
                      </span>
                      {run.operation === "backup" &&
                        run.status === "success" &&
                        run.archivePath && (
                        <>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={t("backups.restore")}
                            onClick={() => handleRestore(run)}
                            disabled={restoreBackup.isPending}
                          >
                            <IconRestore className="size-4" />
                          </Button>
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
                        </>
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
      {restoreTarget && (
        <Dialog
          open={!!restoreTarget}
          onOpenChange={(open) => {
            if (!open) {
              setRestoreTarget(null);
              setRestoreSecretKey("");
              setRestoreSelectedItems([]);
            }
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("backups.restoreTitle")}</DialogTitle>
              <DialogDescription>
                {t("backups.restoreDescription")}
              </DialogDescription>
            </DialogHeader>
            <DialogBody className="space-y-4">
              {restoreTarget.requiresSecretKey && (
                <div>
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    {t("backups.restoreUploadKey")}
                  </label>
                  <Input
                    type="password"
                    value={restoreSecretKey}
                    onChange={(event) =>
                      setRestoreSecretKey(event.target.value)
                    }
                    placeholder={t("backups.restoreSecretPlaceholder")}
                    autoComplete="off"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("backups.restoreSecretDescription")}
                  </p>
                </div>
              )}
              <div className="space-y-2">
                <p className="text-xs font-medium uppercase text-muted-foreground">
                  {t("backups.restoreChoices")}
                </p>
                {!restoreOptionsEnabled && (
                  <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
                    {t("backups.restoreChoicesNeedKey")}
                  </p>
                )}
                {restoreOptionsEnabled && restoreOptionsLoading && (
                  <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
                    <Spinner size="sm" />
                    {t("backups.restoreChoicesLoading")}
                  </div>
                )}
                {restoreOptionsError && (
                  <p className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
                    {t("backups.restoreChoicesFailed")}
                  </p>
                )}
                {restoreOptions && restoreOptions.items.length === 0 && (
                  <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
                    {t("backups.restoreChoicesEmpty")}
                  </p>
                )}
                {restoreOptions && restoreOptions.items.length > 0 && (
                  <div className="space-y-2 rounded-lg border border-border bg-background p-3">
                    {restoreOptions.items.map((item) => (
                      <label
                        key={item.key}
                        className="flex items-start gap-3 rounded-md p-2 transition-colors hover:bg-muted/50"
                      >
                        <Checkbox
                          checked={restoreSelectedItems.includes(item.key)}
                          disabled={item.required || restoreBackup.isPending}
                          onCheckedChange={(checked) =>
                            toggleRestoreItem(item.key, checked === true)
                          }
                          aria-label={item.label}
                          className="mt-0.5"
                        />
                        <span className="min-w-0">
                          <span className="block text-sm font-medium text-foreground">
                            {item.label}
                          </span>
                          <span className="block text-xs text-muted-foreground">
                            {item.description}
                          </span>
                        </span>
                      </label>
                    ))}
                  </div>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                {t("backups.restoreWarning")}
              </p>
            </DialogBody>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setRestoreTarget(null);
                  setRestoreSecretKey("");
                  setRestoreSelectedItems([]);
                }}
              >
                {t("backups.cancelRestore")}
              </Button>
              <Button
                variant="destructive"
                onClick={restoreWithCurrentKey}
                disabled={
                  restoreBackup.isPending ||
                  restoreOptionsLoading ||
                  restoreSelectedItems.length === 0 ||
                  (restoreTarget.requiresSecretKey &&
                    restoreSecretKey.trim() === "")
                }
              >
                <IconRestore className="size-4" />
                {restoreBackup.isPending
                  ? t("backups.restoring")
                  : t("backups.restore")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
      <Dialog
        open={uploadRestoreOpen}
        onOpenChange={(open) => {
          if (!open) closeUploadRestoreDialog();
          if (open) setUploadRestoreOpen(true);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("backups.restoreUploadTitle")}</DialogTitle>
            <DialogDescription>
              {t("backups.restoreUploadDescription")}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-4">
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
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={closeUploadRestoreDialog}>
              {t("backups.cancelRestore")}
            </Button>
            <Button
              variant="destructive"
              onClick={restoreUploadedBackup}
              disabled={uploadRestoreBackup.isPending || !uploadRestoreFile}
            >
              <IconRestore className="size-4" />
              {uploadRestoreBackup.isPending
                ? t("backups.restoring")
                : t("backups.restore")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
