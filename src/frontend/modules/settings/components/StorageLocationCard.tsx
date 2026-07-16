// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconDatabaseImport,
  IconExternalLink,
  IconPencil,
  IconPlugConnected,
  IconTrash,
} from "@tabler/icons-react";
import { Badge } from "@resources/components/ui/Badge";
import { Button } from "@resources/components/ui/Button";
import { ConfirmDialog } from "@resources/components/ui/ConfirmDialog";
import { Switch } from "@resources/components/ui/Switch";
import {
  type StorageLocation,
  useAuthorizeStorageLocation,
  useDeleteStorageLocation,
  useMigrateStorageLocationBackups,
  useTestStorageLocation,
  useUpdateStorageLocation,
} from "../hooks/useStorageLocations";
import { storageProvider } from "./storage-location-options";
import { StorageLocationTestDialog } from "./StorageLocationTestDialog";

type StorageLocationCardProps = {
  location: StorageLocation;
  onEdit: (location: StorageLocation) => void;
};

function locationSummary(location: StorageLocation) {
  if (location.locationType === "local") {
    return location.basePath ?? "";
  }
  if (location.locationType === "aws") {
    return [location.bucket, location.region].filter(Boolean).join(" · ");
  }
  if (location.locationType === "samba") {
    return [location.host, location.shareName].filter(Boolean).join(" · ");
  }
  if (location.locationType === "sharepoint") {
    return [location.siteUrl, location.driveId].filter(Boolean).join(" · ");
  }
  if (
    location.locationType.includes("onedrive") ||
    location.locationType === "google_drive"
  ) {
    return [location.driveId, location.basePath].filter(Boolean).join(" · ");
  }
  return [location.host, location.basePath].filter(Boolean).join(" · ");
}

function requiresConsent(location: StorageLocation) {
  return (
    location.locationType === "google_drive" ||
    location.locationType === "onedrive_personal" ||
    location.locationType === "onedrive_business" ||
    location.locationType === "sharepoint"
  );
}

export function StorageLocationCard({
  location,
  onEdit,
}: StorageLocationCardProps) {
  const { t } = useTranslation("settings");
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [testOpen, setTestOpen] = useState(false);
  const authorizeLocation = useAuthorizeStorageLocation();
  const deleteLocation = useDeleteStorageLocation();
  const migrateBackups = useMigrateStorageLocationBackups();
  const updateLocation = useUpdateStorageLocation();
  const testLocation = useTestStorageLocation();
  const provider = storageProvider(location.locationType);
  const Icon = provider.icon;
  const summary = locationSummary(location) || t("storage.noEndpoint");
  const protectedLocal = location.locationType === "local";

  function handleDelete() {
    deleteLocation.mutate(location.id, {
      onSuccess: () => setDeleteOpen(false),
    });
  }

  function handleAuthorize() {
    authorizeLocation.mutate(location.id, {
      onSuccess: (data) => {
        window.location.assign(data.authorizationUrl);
      },
    });
  }

  function handleMigrateBackups() {
    migrateBackups.mutate(location.id);
  }

  function handleTest() {
    setTestOpen(true);
    testLocation.mutate(location.id);
  }

  return (
    <>
      <div className="flex items-center gap-4 rounded-lg border border-border bg-card p-4">
        <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-muted-foreground">
          <Icon className="size-5" />
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="truncate text-sm font-medium text-foreground">
              {location.name}
            </h4>
            <Badge variant={location.enabled ? "default" : "secondary"}>
              {location.enabled ? t("storage.enabled") : t("storage.disabled")}
            </Badge>
            <Badge variant="outline">{t(provider.labelKey)}</Badge>
          </div>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {summary}
          </p>
        </div>

        <div className="flex items-center gap-1">
          {requiresConsent(location) && (
            <Button
              variant="outline"
              size="sm"
              disabled={authorizeLocation.isPending}
              onClick={handleAuthorize}
            >
              <IconExternalLink className="size-4" />
              {t("storage.connectConsent")}
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            disabled={!location.enabled || testLocation.isPending}
            onClick={handleTest}
            data-testid={`storage-test-${location.id}`}
            aria-label={t("storage.test.button")}
          >
            {testLocation.isPending ? (
              <span className="inline-block size-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
            ) : (
              <IconPlugConnected className="size-4" />
            )}
            {t("storage.test.button")}
          </Button>
          {location.locationType === "local" && (
            <Button
              variant="outline"
              size="sm"
              disabled={!location.enabled || migrateBackups.isPending}
              onClick={handleMigrateBackups}
            >
              <IconDatabaseImport className="size-4" />
              {migrateBackups.isPending
                ? t("storage.migratingBackups")
                : t("storage.migrateBackups")}
            </Button>
          )}
          <Switch
            checked={location.enabled}
            aria-label={t("storage.toggleEnabled")}
            disabled={protectedLocal || updateLocation.isPending}
            onCheckedChange={(enabled) =>
              updateLocation.mutate({ id: location.id, enabled })
            }
          />
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("storage.editLocation")}
            onClick={() => onEdit(location)}
          >
            <IconPencil className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("storage.deleteLocation")}
            disabled={protectedLocal}
            onClick={() => setDeleteOpen(true)}
          >
            <IconTrash className="size-4 text-destructive" />
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("storage.confirmDeleteTitle")}
        description={t("storage.confirmDeleteDescription")}
        confirmLabel={t("storage.confirmDeleteLabel")}
        loading={deleteLocation.isPending}
        onConfirm={handleDelete}
      />

      <StorageLocationTestDialog
        result={testLocation.data ?? null}
        pending={testLocation.isPending}
        locationName={location.name}
        open={testOpen}
        onOpenChange={setTestOpen}
      />
    </>
  );
}
