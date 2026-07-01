// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconCheck,
} from "@tabler/icons-react";
import { Button } from "@resources/components/ui/Button";
import { Checkbox } from "@resources/components/ui/Checkbox";
import { Input } from "@resources/components/ui/Input";
import { Spinner } from "@resources/components/ui/Spinner";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@resources/components/ui/Dialog";
import {
  useCancelContainerBackupRun,
  useContainerBackupRestoreOptions,
  useContainerBackupRun,
  useRestoreContainerBackup,
  type ContainerBackupRun,
} from "../hooks/useContainerBackups";

// Human-readable mapping for restore_* progress stages emitted by
// the backend. The container_backup service writes these to the
// run record throughout the restore pipeline.
const RESTORE_STAGE_LABELS: Record<string, string> = {
  restore_connecting: "backups.progress.restore_connecting",
  restore_inspecting: "backups.progress.restore_inspecting",
  restore_scanning: "backups.progress.restore_scanning",
  restore_image: "backups.progress.restore_image",
  restore_filesystem: "backups.progress.restore_filesystem",
  restore_mounts: "backups.progress.restore_mounts",
};

function describeRestoreStage(
  stage: string | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (!stage) return "";
  const key = RESTORE_STAGE_LABELS[stage];
  if (key) {
    return t(key, { defaultValue: stage });
  }
  return t(`backups.progress.${stage}`, { defaultValue: stage });
}

// RestoreProgressView is the live-progress body of the dialog
// shown after the operator has kicked off a restore. Renders the
// status icon, the human-readable stage / message, and the
// terminal-state copy (success / failure).
export function RestoreProgressView({
  run,
  loading,
  startedAt,
}: {
  run?: ContainerBackupRun;
  loading: boolean;
  startedAt: string;
}) {
  const { t } = useTranslation("containers");
  const isRunning = run?.status === "running";
  const isSuccess = run?.status === "success";
  const isFailure = run?.status === "failure";
  const stageLabel = describeRestoreStage(run?.progressStage, t);
  const message = run?.progressMessage ?? "";
  const startedLabel = useMemo(() => {
    try {
      return new Date(startedAt).toLocaleString();
    } catch {
      return startedAt;
    }
  }, [startedAt]);

  return (
    <div className="space-y-4">
      <div className="flex items-start gap-3 rounded-lg border border-border bg-background p-4">
        {isSuccess ? (
          <IconCheck
            className="mt-0.5 size-5 shrink-0 text-emerald-500"
            aria-hidden="true"
          />
        ) : isFailure ? (
          <IconAlertTriangle
            className="mt-0.5 size-5 shrink-0 text-destructive"
            aria-hidden="true"
          />
        ) : (
          <Spinner size="md" className="mt-0.5 shrink-0" />
        )}
        <div className="min-w-0 flex-1 space-y-1">
          <p className="text-sm font-medium text-foreground">
            {isSuccess
              ? t("backups.restoreCompleteHeading")
              : isFailure
                ? t("backups.restoreFailedHeading")
                : t("backups.restoringHeading")}
          </p>
          <p className="text-xs text-muted-foreground">
            {t("backups.startedAt", { value: startedLabel })}
          </p>
          {isRunning && (
            <div className="space-y-1 pt-1">
              {stageLabel && (
                <p className="text-xs font-medium text-foreground">
                  {stageLabel}
                </p>
              )}
              {message && message !== stageLabel && (
                <p className="break-words text-xs text-muted-foreground">
                  {message}
                </p>
              )}
            </div>
          )}
          {isFailure && run?.error && (
            <p className="break-words text-xs text-destructive">
              {run.error}
            </p>
          )}
          {isSuccess && (
            <p className="text-xs text-emerald-600 dark:text-emerald-400">
              {t("backups.restoreCompleteHint")}
            </p>
          )}
          {loading && !run && (
            <p className="text-xs text-muted-foreground">
              {t("backups.restoreChoicesLoading")}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

type RestoreRunDialogProps = {
  // The original backup run we want to restore. While the restore is
  // running, this stays open and a separate progress view is shown
  // inside the same dialog so the user can keep watching the
  // backup → restore pipeline from one place.
  run: ContainerBackupRun | null;
  // Required because the restore API takes containerId in the path
  // for the existing per-container tab. The cross-environment
  // Backups page passes the run's own containerId here.
  containerId: string;
  onClose: () => void;
};

// RestoreRunDialog is the full restore flow: fetch restore options,
// ask for a secret key when the archive is encrypted, fire the
// restore mutation, and then poll the resulting restore run until
// it reaches a terminal state. Used by both the per-container
// Backups tab and the cross-environment Backups overview page.
export function RestoreRunDialog({
  run,
  containerId,
  onClose,
}: RestoreRunDialogProps) {
  const { t } = useTranslation("containers");
  const restoreBackup = useRestoreContainerBackup(containerId);
  const cancelRun = useCancelContainerBackupRun();
  const [restoreSecretKey, setRestoreSecretKey] = useState("");
  const [restoreSelectedItems, setRestoreSelectedItems] = useState<string[]>(
    [],
  );
  // While the restore is in progress, the mutation returns the new
  // restore run. We track it separately so the dialog knows which
  // phase to render and which run to poll for live progress.
  const [restoreRunId, setRestoreRunId] = useState<string | null>(null);

  // Reset transient state whenever the dialog opens for a new run.
  useEffect(() => {
    if (run) {
      setRestoreSecretKey("");
      setRestoreSelectedItems([]);
      setRestoreRunId(null);
    }
  }, [run?.id]);

  const restoreOptionsEnabled =
    !!run && (!run.requiresSecretKey || restoreSecretKey.trim() !== "");
  const {
    data: restoreOptions,
    isFetching: restoreOptionsLoading,
    isError: restoreOptionsError,
  } = useContainerBackupRestoreOptions(
    run?.id,
    restoreSecretKey,
    restoreOptionsEnabled,
  );
  const { data: restoreRun, isLoading: restoreRunLoading } =
    useContainerBackupRun(containerId, restoreRunId ?? undefined);
  const restoreIsActive = restoreRun?.status === "running";

  useEffect(() => {
    if (!restoreOptions) return;
    setRestoreSelectedItems(
      restoreOptions.items
        .filter((item) => item.default || item.required)
        .map((item) => item.key),
    );
  }, [restoreOptions]);

  function closeRestoreDialog() {
    onClose();
    setRestoreRunId(null);
    setRestoreSecretKey("");
    setRestoreSelectedItems([]);
  }

  function toggleRestoreItem(key: string, checked: boolean) {
    setRestoreSelectedItems((current) => {
      if (checked) {
        return current.includes(key) ? current : [...current, key];
      }
      return current.filter((item) => item !== key);
    });
  }

  function startRestore() {
    if (!run) return;
    restoreBackup.mutate(
      {
        id: run.id,
        secretKey: restoreSecretKey,
        restoreItems: restoreSelectedItems,
      },
      {
        onSuccess: (newRun) => {
          setRestoreRunId(newRun.id);
        },
      },
    );
  }

  return (
    <Dialog
      open={!!run}
      onOpenChange={(open) => {
        // While a restore is running, ignore outside clicks so the
        // user keeps the progress view. They can still press the
        // Close button once the run reaches a terminal state.
        if (!open && (restoreIsActive || restoreRunId === null)) {
          closeRestoreDialog();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          {restoreRunId ? (
            <DialogTitle>
              {restoreRun?.status === "failure"
                ? t("backups.restoreFailedTitle")
                : restoreIsActive
                  ? t("backups.restoringTitle")
                  : t("backups.restoreCompleteTitle")}
            </DialogTitle>
          ) : (
            <DialogTitle>{t("backups.restoreTitle")}</DialogTitle>
          )}
          {restoreRunId ? null : (
            <DialogDescription>
              {t("backups.restoreDescription")}
            </DialogDescription>
          )}
        </DialogHeader>
        <DialogBody className="space-y-4">
          {restoreRunId ? (
            <RestoreProgressView
              run={restoreRun}
              loading={restoreRunLoading}
              startedAt={run?.startedAt ?? new Date().toISOString()}
            />
          ) : (
            <>
              {run?.requiresSecretKey && (
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
                          disabled={
                            item.required || restoreBackup.isPending
                          }
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
            </>
          )}
        </DialogBody>
        <DialogFooter>
          {restoreRunId ? (
            <>
              {restoreIsActive && (
                <Button
                  variant="destructive"
                  onClick={() => {
                    if (restoreRun) {
                      cancelRun.mutate(restoreRun.id);
                    }
                  }}
                  disabled={cancelRun.isPending}
                  data-testid="restore-cancel"
                >
                  {t("backups.cancelRunConfirm")}
                </Button>
              )}
              <Button
                onClick={closeRestoreDialog}
                disabled={restoreIsActive}
                data-testid="restore-close"
              >
                {restoreIsActive
                  ? t("backups.restoring")
                  : t("backups.closeRestore")}
              </Button>
            </>
          ) : (
            <>
              <Button
                variant="outline"
                onClick={closeRestoreDialog}
                disabled={restoreBackup.isPending}
              >
                {t("backups.cancelRestore")}
              </Button>
              <Button
                variant="destructive"
                onClick={startRestore}
                disabled={
                  restoreBackup.isPending ||
                  restoreOptionsLoading ||
                  restoreSelectedItems.length === 0 ||
                  (run?.requiresSecretKey &&
                    restoreSecretKey.trim() === "")
                }
              >
                {restoreBackup.isPending
                  ? t("backups.restoring")
                  : t("backups.restore")}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}