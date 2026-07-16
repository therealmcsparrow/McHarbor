// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import {
  IconArchive,
  IconCalendarEvent,
  IconClock,
  IconDownload,
  IconExternalLink,
  IconLink,
  IconPencil,
  IconRestore,
  IconTrash,
} from "@tabler/icons-react";
import { Badge } from "@resources/components/ui/Badge";
import { Button } from "@resources/components/ui/Button";
import { Checkbox } from "@resources/components/ui/Checkbox";
import { Spinner } from "@resources/components/ui/Spinner";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@resources/components/ui/Tooltip";
import { describeCron } from "@resources/utils/schedule";
import { cn } from "@resources/utils/cn";
import { useLocaleFormat } from "@resources/hooks/useLocaleFormat";
import {
  type ContainerBackupPlan,
  type ContainerBackupRun,
} from "../hooks/useContainerBackups";
import { type StorageLocationType } from "../../settings/hooks/useStorageLocations";
import { storageProvider } from "../../settings/components/storage-location-options";
import {
  destinationLabel,
  environmentName,
  formatBytes,
  formatTimestamp,
  statusBadgeVariant,
} from "./backups-page-helpers";

export function PlanBulkToolbar({
  selected,
  total,
  onClearSelection,
  onBulkDelete,
  deleting,
}: {
  selected: ContainerBackupPlan[];
  total: number;
  onClearSelection: () => void;
  onBulkDelete: () => void;
  deleting: boolean;
}) {
  const { t } = useTranslation("containers");
  if (selected.length === 0) return null;
  return (
    <div
      className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-2 text-sm"
      data-testid="plans-bulk-toolbar"
    >
      <div className="flex items-center gap-2 text-foreground">
        <span className="font-medium">{t("backups.bulkSelected", { count: selected.length })}</span>
        <span className="text-xs text-muted-foreground">
          {t("backups.bulkOfTotal", { total })}
        </span>
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={onClearSelection}
          disabled={deleting}
        >
          {t("backups.bulkClear")}
        </Button>
        <Button
          variant="destructive"
          size="sm"
          onClick={onBulkDelete}
          disabled={deleting}
          data-testid="plans-bulk-delete"
        >
          <IconTrash className="size-4" aria-hidden="true" />
          {t("backups.bulkDelete")}
        </Button>
      </div>
    </div>
  );
}

export function RunsBulkToolbar({
  selected,
  total,
  onClearSelection,
  onBulkDownload,
  onBulkDelete,
  deleting,
}: {
  selected: ContainerBackupRun[];
  total: number;
  onClearSelection: () => void;
  onBulkDownload: () => void;
  onBulkDelete: () => void;
  deleting: boolean;
}) {
  const { t } = useTranslation("containers");
  if (selected.length === 0) return null;
  // Only runs that finished successfully and have an archive on disk
  // can be downloaded; the rest get filtered out by the helper.
  const downloadable = selected.filter(
    (run) =>
      run.operation === "backup" &&
      run.status === "success" &&
      !!run.archivePath,
  ).length;
  return (
    <div
      className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-2 text-sm"
      data-testid="runs-bulk-toolbar"
    >
      <div className="flex items-center gap-2 text-foreground">
        <span className="font-medium">
          {t("backups.bulkRunsSelected", { count: selected.length })}
        </span>
        <span className="text-xs text-muted-foreground">
          {t("backups.bulkOfTotal", { total })}
        </span>
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={onClearSelection}
          disabled={deleting}
        >
          {t("backups.bulkClear")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onBulkDownload}
          disabled={deleting || downloadable === 0}
          data-testid="runs-bulk-download"
        >
          <IconDownload className="size-4" aria-hidden="true" />
          {t("backups.bulkRunsDownload", { count: downloadable })}
        </Button>
        <Button
          variant="destructive"
          size="sm"
          onClick={onBulkDelete}
          disabled={deleting}
          data-testid="runs-bulk-delete"
        >
          <IconTrash className="size-4" aria-hidden="true" />
          {t("backups.bulkDelete")}
        </Button>
      </div>
    </div>
  );
}

export function BackupRunsTable({
  runs,
  loading,
  envNameById,
  selectedIds,
  onToggleSelected,
  onToggleAll,
  onRestore,
  onDelete,
  onDownload,
  onOpenContainer,
}: {
  runs: ContainerBackupRun[];
  loading: boolean;
  envNameById: Map<string, string>;
  selectedIds: Set<string>;
  onToggleSelected: (runId: string) => void;
  onToggleAll: () => void;
  onRestore: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  onDownload: (run: ContainerBackupRun) => void;
  onOpenContainer: (environmentId: string, containerId: string) => void;
}) {
  const { t } = useTranslation("containers");
  const { t: tc } = useTranslation("common");
  const formatPrefs = useLocaleFormat();
  const allSelected = runs.length > 0 && selectedIds.size === runs.length;
  const someSelected = selectedIds.size > 0 && selectedIds.size < runs.length;
  if (loading && runs.length === 0) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size="lg" />
      </div>
    );
  }
  if (runs.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border bg-card/40 p-10 text-center">
        <IconArchive className="mx-auto mb-3 size-8 text-muted-foreground" />
        <p className="text-sm font-medium text-foreground">
          {t("backups.runs.emptyTitle")}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">
          {t("backups.runs.emptyHint")}
        </p>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10 bg-muted">
            <tr className="text-left text-xs uppercase tracking-wide text-muted-foreground">
              <th className="w-10 px-3 py-3">
                <Checkbox
                  checked={
                    allSelected ? true : someSelected ? "indeterminate" : false
                  }
                  onCheckedChange={onToggleAll}
                  aria-label={t("backups.bulkSelectAllRuns")}
                />
              </th>
              <th className="px-4 py-3 font-medium">{t("backups.runs.table.container")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.runs.table.environment")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.runs.table.status")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.runs.table.destinations")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.runs.table.size")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.runs.table.startedAt")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.runs.table.duration")}</th>
              <th className="px-4 py-3 font-medium text-right">
                {tc("common.actions")}
              </th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => {
              const envName = environmentName(
                run.environmentId,
                envNameById.get(run.environmentId) ?? run.environmentId.slice(0, 8),
              );
              const destinations = run.destinations ?? [];
              const hasSuccess = destinations.some((d) => d.status === "success");
              const hasFailure = destinations.some((d) => d.status === "failure");
              const hasUploading = destinations.some((d) => d.status === "uploading");
              const canRestore =
                run.operation === "backup" && run.status === "success";
              const canDownload =
                run.operation === "backup" &&
                run.status === "success" &&
                !!run.archivePath;
              const isSelected = selectedIds.has(run.id);
              return (
                <tr
                  key={run.id}
                  className="border-t border-border align-top transition-colors hover:bg-muted/30 data-[selected=true]:bg-primary/5"
                  data-selected={isSelected || undefined}
                  data-testid={`run-row-${run.id}`}
                >
                  <td className="px-3 py-3">
                    <Checkbox
                      checked={isSelected}
                      onCheckedChange={() => onToggleSelected(run.id)}
                      aria-label={t("backups.bulkSelectRunRow", {
                        name:
                          run.containerName ??
                          run.containerId.slice(0, 12),
                      })}
                    />
                  </td>
                  <td className="px-4 py-3">
                    <Link
                      to={`/containers/${run.containerId}`}
                      onClick={(event) => {
                        // Switch to the run's owning environment
                        // before navigating so the detail page
                        // queries the right orchestrator.
                        event.preventDefault();
                        onOpenContainer(run.environmentId, run.containerId);
                      }}
                      className="flex flex-col items-start gap-1 font-medium text-foreground hover:text-primary"
                    >
                      <span className="flex items-center gap-1">
                        {run.containerName || run.containerId.slice(0, 12)}
                        <IconExternalLink className="size-3.5 opacity-60" aria-hidden="true" />
                      </span>
                      {run.containerIdStale && (
                        <StaleIdBadge />
                      )}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">{envName}</td>
                  <td className="px-4 py-3">
                    <Badge variant={statusBadgeVariant(run.status)}>
                      {run.status === "running" && <Spinner size="sm" className="mr-1.5" />}
                      {t(`backups.status.${run.status}`)}
                    </Badge>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap items-center gap-1.5">
                      {destinations.length === 0 ? (
                        <span className="text-xs text-muted-foreground">—</span>
                      ) : (
                        destinations.map((destination) => {
                          const name =
                            destination.storageLocationName ||
                            destinationLabel(destination, "—");
                          const provider = storageProvider(
                            (destination.locationType || "local") as StorageLocationType,
                          );
                          const DestIcon = provider.icon;
                          const tint =
                            destination.status === "success"
                              ? "text-emerald-500"
                              : destination.status === "failure"
                                ? "text-destructive"
                                : "text-muted-foreground";
                          return (
                            <Tooltip key={destination.id}>
                              <TooltipTrigger asChild>
                                <span
                                  className={cn(
                                    "inline-flex size-7 cursor-default items-center justify-center rounded-md border border-border bg-background hover:bg-muted/50",
                                  )}
                                  aria-label={`${destination.status}: ${name}`}
                                >
                                  <DestIcon
                                    className={cn("size-4", tint)}
                                    aria-hidden="true"
                                  />
                                </span>
                              </TooltipTrigger>
                              <TooltipContent>
                                <div className="space-y-0.5">
                                  <div className="font-medium">{name}</div>
                                  <div className="text-[10px] text-muted-foreground">
                                    {t(
                                      `backups.storageStatus.${destination.status}`,
                                    )}
                                  </div>
                                </div>
                              </TooltipContent>
                            </Tooltip>
                          );
                        })
                      )}
                      {hasUploading && (
                        <span className="ml-1 text-[10px] text-muted-foreground">
                          {hasSuccess ? "" : t("backups.runs.table.runningHint")}
                        </span>
                      )}
                      {hasFailure && !hasSuccess && (
                        <span className="ml-1 text-[10px] text-destructive">
                          {t("backups.runs.table.failedHint")}
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                    {formatBytes(run.archiveSize)}
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {formatTimestamp(run.startedAt, formatPrefs)}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                    {run.durationMs > 0
                      ? `${Math.round(run.durationMs / 1000)}s`
                      : "—"}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.runs.table.restore")}
                        onClick={() => onRestore(run)}
                        disabled={!canRestore}
                        data-testid={`run-restore-${run.id}`}
                      >
                        <IconRestore className="size-4" aria-hidden="true" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.runs.table.download")}
                        disabled={!canDownload}
                        onClick={() => onDownload(run)}
                        data-testid={`run-download-${run.id}`}
                      >
                        <IconDownload className="size-4" aria-hidden="true" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.runs.table.delete")}
                        onClick={() => onDelete(run)}
                        data-testid={`run-delete-${run.id}`}
                      >
                        <IconTrash
                          className="size-4 text-destructive"
                          aria-hidden="true"
                        />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.runs.table.open")}
                        onClick={() =>
                          onOpenContainer(run.environmentId, run.containerId)
                        }
                      >
                        <IconExternalLink
                          className="size-4"
                          aria-hidden="true"
                        />
                      </Button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function BackupPlansTable({
  plans,
  loading,
  envNameById,
  selectedIds,
  onToggleSelected,
  onToggleAll,
  onEdit,
  onDelete,
  onOpenContainer,
}: {
  plans: ContainerBackupPlan[];
  loading: boolean;
  envNameById: Map<string, string>;
  selectedIds: Set<string>;
  onToggleSelected: (planId: string) => void;
  onToggleAll: () => void;
  onEdit: (plan: ContainerBackupPlan) => void;
  onDelete: (plan: ContainerBackupPlan) => void;
  onOpenContainer: (environmentId: string, containerId: string) => void;
}) {
  const { t } = useTranslation("containers");
  const { t: tc } = useTranslation("common");
  const allSelected =
    plans.length > 0 && selectedIds.size === plans.length;
  const someSelected =
    selectedIds.size > 0 && selectedIds.size < plans.length;
  const formatPrefs = useLocaleFormat();
  if (loading && plans.length === 0) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size="lg" />
      </div>
    );
  }
  if (plans.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border bg-card/40 p-10 text-center">
        <IconCalendarEvent className="mx-auto mb-3 size-8 text-muted-foreground" />
        <p className="text-sm font-medium text-foreground">
          {t("backups.plans.emptyTitle")}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">
          {t("backups.plans.emptyHint")}
        </p>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10 bg-muted">
            <tr className="text-left text-xs uppercase tracking-wide text-muted-foreground">
              <th className="w-10 px-3 py-3">
                <Checkbox
                  // Radix's Checkbox supports the special
                  // "indeterminate" checked value to render the
                  // dash icon — the rest of the indeterminate
                  // styling (data-state="indeterminate" classes
                  // on the wrapper) lights up automatically.
                  checked={
                    allSelected
                      ? true
                      : someSelected
                        ? "indeterminate"
                        : false
                  }
                  onCheckedChange={onToggleAll}
                  aria-label={t("backups.bulkSelectAll")}
                />
              </th>
              <th className="px-4 py-3 font-medium">{t("backups.plans.table.name")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.plans.table.environment")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.plans.table.container")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.plans.table.schedule")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.plans.table.destinations")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.plans.table.retention")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.plans.table.nextRun")}</th>
              <th className="px-4 py-3 font-medium">{t("backups.plans.table.enabled")}</th>
              <th className="px-4 py-3 font-medium text-right">
                {tc("common.actions")}
              </th>
            </tr>
          </thead>
          <tbody>
            {plans.map((plan) => {
              const envName = environmentName(
                plan.environmentId,
                envNameById.get(plan.environmentId) ?? plan.environmentId.slice(0, 8),
              );
              const isSelected = selectedIds.has(plan.id);
              return (
                <tr
                  key={plan.id}
                  className="border-t border-border align-top transition-colors hover:bg-muted/30 data-[selected=true]:bg-primary/5"
                  data-selected={isSelected || undefined}
                  data-testid={`plan-row-${plan.id}`}
                >
                  <td className="px-3 py-3">
                    <Checkbox
                      checked={isSelected}
                      onCheckedChange={() => onToggleSelected(plan.id)}
                      aria-label={t("backups.bulkSelectRow", { name: plan.name })}
                    />
                  </td>
                  <td className="px-4 py-3 font-medium text-foreground">
                    {plan.name}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">{envName}</td>
                  <td className="px-4 py-3">
                    <Link
                      to={`/containers/${plan.containerId}`}
                      onClick={(event) => {
                        event.preventDefault();
                        onOpenContainer(plan.environmentId, plan.containerId);
                      }}
                      className="flex flex-col items-start gap-1 text-foreground hover:text-primary"
                    >
                      <span className="flex items-center gap-1">
                        {plan.containerName || plan.containerId.slice(0, 12)}
                        <IconExternalLink className="size-3.5 opacity-60" aria-hidden="true" />
                      </span>
                      {plan.containerIdStale && <StaleIdBadge />}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {plan.cron ? (
                      <span className="flex items-center gap-1">
                        <IconClock className="size-3.5 opacity-70" aria-hidden="true" />
                        {describeCron(plan.cron, tc)}
                      </span>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {plan.storageLocationIds.length > 0
                      ? t("backups.destinationsCount", {
                          count: plan.storageLocationIds.length,
                        })
                      : "—"}
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {retentionText(plan, t)}
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {formatTimestamp(plan.nextRunAt, formatPrefs)}
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant={plan.enabled ? "default" : "secondary"}>
                      {plan.enabled
                        ? t("backups.enabled")
                        : t("backups.disabled")}
                    </Badge>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.plans.table.edit")}
                        onClick={() => onEdit(plan)}
                        data-testid={`plan-edit-${plan.id}`}
                      >
                        <IconPencil className="size-4" aria-hidden="true" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.plans.table.delete")}
                        onClick={() => onDelete(plan)}
                        data-testid={`plan-delete-${plan.id}`}
                      >
                        <IconTrash className="size-4 text-destructive" aria-hidden="true" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("backups.plans.table.open")}
                        onClick={() =>
                          onOpenContainer(plan.environmentId, plan.containerId)
                        }
                      >
                        <IconExternalLink className="size-4" aria-hidden="true" />
                      </Button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function retentionText(
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

// StaleIdBadge surfaces the containerIdStale flag the server
// sets when the auto-refresh on read changed the container_id
// because the stored id no longer matched a live container. The
// persisted row has already been updated by the time the flag is
// set, so the UI just shows a small "Re-linked" badge to make the
// re-link visible to the operator.
export function StaleIdBadge() {
  const { t } = useTranslation("containers");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-amber-600 dark:text-amber-400"
          data-testid="stale-id-badge"
        >
          <IconLink className="size-3" aria-hidden="true" />
          {t("backups.staleIdRelinked")}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        {t("backups.staleIdTooltip")}
      </TooltipContent>
    </Tooltip>
  );
}
