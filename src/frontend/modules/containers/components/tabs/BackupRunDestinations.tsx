// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconCircleCheckFilled,
  IconCircleXFilled,
  IconDownload,
  IconHistory,
  IconRefresh,
  IconTrash,
} from "@tabler/icons-react";
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
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Spinner } from "@resources/components/ui/Spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@resources/components/ui/Tooltip";
import { cn } from "@resources/utils/cn";
import {
  containerBackupDownloadUrl,
  type ContainerBackupRun,
  type ContainerBackupRunDestination,
} from "../../hooks/useContainerBackups";
import {
  type StorageLocationType,
} from "../../../settings/hooks/useStorageLocations";
import { storageProvider } from "../../../settings/components/storage-location-options";

type BackupRunDestinationsProps = {
  run: ContainerBackupRun;
  onRestore: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  onRetryUpload: (destination: ContainerBackupRunDestination) => void;
  retryPendingFor?: string;
};

function destinationLabel(
  destination: ContainerBackupRunDestination,
  fallback: string,
) {
  if (destination.storageLocationName) return destination.storageLocationName;
  if (destination.storageLocationId) return destination.storageLocationId;
  return fallback;
}

// DestinationIcon renders the storage-type glyph as the primary icon
// (folder for local, OneDrive cloud, SharePoint office, AWS, Google
// Drive, etc.) with a small status badge in the corner so the
// success / failure / uploading signal is visible at a glance. The
// badge uses a card-colored background so it visually detaches from
// the underlying icon. For uploading destinations the badge is a
// spinning ring so the operator can still see the storage type
// underneath.
function DestinationIcon({
  destination,
}: {
  destination: ContainerBackupRunDestination;
}) {
  const { t } = useTranslation("containers");
  const provider = storageProvider(
    (destination.locationType || "local") as StorageLocationType,
  );
  const Icon = provider.icon;
  const tintClass =
    destination.status === "success"
      ? "text-emerald-500"
      : destination.status === "uploading"
        ? "text-primary"
        : "text-destructive";
  return (
    <span
      className="relative inline-flex items-center justify-center"
      aria-label={`${destination.locationType || "storage"} ${destination.status}`}
    >
      <Icon className={cn("size-5", tintClass)} aria-hidden="true" />
      <span
        className={cn(
          "absolute -bottom-0.5 -right-0.5 inline-flex size-4 items-center justify-center rounded-full bg-card",
        )}
      >
        {destination.status === "success" ? (
          <IconCircleCheckFilled
            className="size-3 text-emerald-500"
            aria-label={t("backups.storageStatus.success")}
          />
        ) : destination.status === "failure" ? (
          <IconCircleXFilled
            className="size-3 text-destructive"
            aria-label={t("backups.storageStatus.failure")}
          />
        ) : (
          <Spinner
            size="sm"
            className="text-primary"
            aria-label={t("backups.storageStatus.uploading")}
          />
        )}
      </span>
    </span>
  );
}

function SuccessDestinationButton({
  destination,
  run,
  onRestore,
  onDelete,
  canDownload,
}: {
  destination: ContainerBackupRunDestination;
  run: ContainerBackupRun;
  onRestore: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  canDownload: boolean;
}) {
  const { t } = useTranslation("containers");
  const name = destinationLabel(destination, t("backups.storage.external"));
  const downloadUrl = canDownload ? containerBackupDownloadUrl(run.id) : null;

  return (
    <DropdownMenu.Root>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenu.Trigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`${t("backups.storageStatus.success")}: ${name}`}
              data-testid={`destination-success-${destination.id}`}
              className="size-7 hover:bg-emerald-500/10"
            >
              <DestinationIcon destination={destination} />
            </Button>
          </DropdownMenu.Trigger>
        </TooltipTrigger>
        <TooltipContent>{name}</TooltipContent>
      </Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="start"
          sideOffset={4}
          className="z-50 min-w-[12rem] overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95"
        >
          <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
            {name}
          </div>
          <DropdownMenu.Separator className="my-1 h-px bg-border" />
          {downloadUrl && (
            <DropdownMenu.Item
              className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none transition-colors focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
              onSelect={() => {
                // Trigger the download by creating a transient anchor.
                // Using onSelect keeps the menu dismissable in a single
                // interaction instead of leaving it open after a click.
                const link = document.createElement("a");
                link.href = downloadUrl;
                link.download = "";
                link.rel = "noopener";
                document.body.appendChild(link);
                link.click();
                link.remove();
              }}
            >
              <IconDownload className="size-4" aria-hidden="true" />
              {t("backups.destinationMenu.download")}
            </DropdownMenu.Item>
          )}
          <DropdownMenu.Item
            className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none transition-colors focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
            onSelect={() => onRestore(run)}
          >
            <IconHistory className="size-4" aria-hidden="true" />
            {t("backups.destinationMenu.restore")}
          </DropdownMenu.Item>
          <DropdownMenu.Item
            className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-destructive outline-none transition-colors focus:bg-destructive/10 data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
            onSelect={() => onDelete(run)}
          >
            <IconTrash className="size-4" aria-hidden="true" />
            {t("backups.destinationMenu.delete")}
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

function FailureDestinationButton({
  destination,
  onRetryUpload,
  retryPending,
}: {
  destination: ContainerBackupRunDestination;
  onRetryUpload: (destination: ContainerBackupRunDestination) => void;
  retryPending: boolean;
}) {
  const { t } = useTranslation("containers");
  const [open, setOpen] = useState(false);
  const name = destinationLabel(destination, t("backups.storage.external"));

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`${t("backups.storageStatus.failure")}: ${name}`}
            data-testid={`destination-failure-${destination.id}`}
            onClick={() => setOpen(true)}
            className="size-7 hover:bg-destructive/10"
          >
            <DestinationIcon destination={destination} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{name}</TooltipContent>
      </Tooltip>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <IconAlertTriangle
              className="size-5 text-destructive"
              aria-hidden="true"
            />
            <DialogTitle>{t("backups.destinationFailure.title")}</DialogTitle>
          </div>
          <DialogDescription>{name}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
            {destination.error || t("backups.destinationFailure.unknownError")}
          </div>
          <p className="text-xs text-muted-foreground">
            {t("backups.destinationFailure.retryHint")}
          </p>
        </DialogBody>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => setOpen(false)}
            data-testid="destination-failure-close"
          >
            {t("backups.destinationFailure.close")}
          </Button>
          <Button
            onClick={() => {
              setOpen(false);
              onRetryUpload(destination);
            }}
            disabled={retryPending}
            data-testid="destination-failure-retry"
          >
            <IconRefresh className="size-4" aria-hidden="true" />
            {t("backups.destinationFailure.retry")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function BackupRunDestinations({
  run,
  onRestore,
  onDelete,
  onRetryUpload,
  retryPendingFor,
}: BackupRunDestinationsProps) {
  const { t } = useTranslation("containers");
  const destinations = run.destinations ?? [];
  if (destinations.length === 0) return null;
  // Only completed backup runs with a downloadable archive expose the
  // Download menu item. Restore runs reuse the same destination row
  // pattern but have no archive to fetch.
  const canDownload =
    run.operation === "backup" && run.status === "success" && !!run.archivePath;

  return (
    <div
      className={cn("mt-2 flex flex-wrap items-center gap-2")}
      data-testid={`destinations-${run.id}`}
    >
      {destinations.map((destination) => {
        if (destination.status === "success") {
          return (
            <SuccessDestinationButton
              key={destination.id}
              destination={destination}
              run={run}
              onRestore={onRestore}
              onDelete={onDelete}
              canDownload={canDownload}
            />
          );
        }
        if (destination.status === "failure") {
          return (
            <FailureDestinationButton
              key={destination.id}
              destination={destination}
              onRetryUpload={onRetryUpload}
              retryPending={retryPendingFor === destination.id}
            />
          );
        }
        // uploading — render the icon + a compact progress bar so
        // the operator sees byte progress next to the destination
        // while the upload goroutine runs. Bytes flow in via the
        // regular runs-query polling loop.
        return (
          <UploadingDestinationItem
            key={destination.id}
            destination={destination}
            label={t("backups.destinationFailure.retryUploading")}
          />
        );
      })}
    </div>
  );
}

// UploadingDestinationItem shows the storage icon + a compact
// progress bar that tracks bytes_uploaded / bytes_total. The bytes
// are populated by the existing per-destination progress callback
// (storage_upload.go) and surface in the runs query, which the
// parent already polls. Renders the storage-type icon (so the
// operator can see which destination is uploading) plus a width
// that adjusts to the available space, with both byte counts and
// percentage visible.
function UploadingDestinationItem({
  destination,
  label,
}: {
  destination: ContainerBackupRunDestination;
  label: string;
}) {
  const { t } = useTranslation("containers");
  const name = destinationLabel(destination, t("backups.storage.external"));
  const uploaded = destination.bytesUploaded ?? 0;
  const total = destination.bytesTotal ?? 0;
  const percent = total > 0 ? Math.min(100, Math.round((uploaded / total) * 100)) : 0;
  const byteText = total > 0
    ? `${formatProgressBytes(uploaded)} / ${formatProgressBytes(total)}`
    : formatProgressBytes(uploaded);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          className="inline-flex min-w-[180px] max-w-[260px] flex-1 items-center gap-2 rounded-md border border-border bg-muted/40 px-2 py-1"
          data-testid={`destination-uploading-${destination.id}`}
          aria-label={`${label}: ${name}`}
        >
          <DestinationIcon destination={destination} />
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <div className="flex items-center justify-between gap-2 text-[11px] leading-none text-muted-foreground">
              <span className="truncate font-medium text-foreground">
                {name}
              </span>
              <span className="font-mono tabular-nums">
                {percent}%
              </span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-full bg-muted">
              <div
                className="h-full bg-primary transition-[width] duration-300 ease-out"
                style={{ width: `${percent}%` }}
              />
            </div>
            <span className="font-mono text-[10px] tabular-nums text-muted-foreground">
              {byteText}
            </span>
          </div>
        </div>
      </TooltipTrigger>
      <TooltipContent>{name}</TooltipContent>
    </Tooltip>
  );
}

function formatProgressBytes(value: number) {
  if (!value) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length - 1,
  );
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}