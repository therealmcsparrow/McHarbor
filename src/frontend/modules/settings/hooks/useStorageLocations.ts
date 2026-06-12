// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "@core/api/client";
import { assertSuccess } from "@resources/utils/api-mutation";

export type StorageLocationType =
  | "local"
  | "ftp"
  | "ftps"
  | "sftp"
  | "samba"
  | "aws"
  | "google_drive"
  | "onedrive_personal"
  | "onedrive_business"
  | "sharepoint";

export type StorageLocation = {
  id: string;
  name: string;
  locationType: StorageLocationType;
  enabled: boolean;
  host?: string;
  port?: number;
  basePath?: string;
  region?: string;
  bucket?: string;
  endpoint?: string;
  tenantId?: string;
  siteUrl?: string;
  driveId?: string;
  shareName?: string;
  domain?: string;
  username?: string;
  authMethod?: "password" | "private_key" | "password_private_key";
  tlsMode?: "explicit" | "implicit";
  passiveMode: boolean;
  createdAt: string;
  updatedAt: string;
};

export type StorageLocationInput = {
  name: string;
  locationType: StorageLocationType;
  enabled?: boolean;
  host?: string;
  port?: number;
  basePath?: string;
  region?: string;
  bucket?: string;
  endpoint?: string;
  tenantId?: string;
  siteUrl?: string;
  driveId?: string;
  shareName?: string;
  domain?: string;
  username?: string;
  authMethod?: "password" | "private_key" | "password_private_key";
  tlsMode?: "explicit" | "implicit";
  passiveMode?: boolean;
  password?: string;
  privateKey?: string;
  passphrase?: string;
  caCertificate?: string;
  clientCertificate?: string;
  clientKey?: string;
  accessKeyId?: string;
  secretAccessKey?: string;
  clientId?: string;
  clientSecret?: string;
  refreshToken?: string;
  token?: string;
};

export type StorageOAuthAuthorizeResponse = {
  authorizationUrl: string;
  expiresAt: string;
};

export type StorageLocationBackupMigrationResult = {
  storageLocationId: string;
  total: number;
  migrated: number;
  skipped: number;
  failed: number;
};

export type BackupEncryptionKeyResponse = {
  key: string;
  encoding: string;
  secret: string;
  secretKey: string;
  secretPath: string;
  setupShell: string;
  setupCommand: string;
};

export type BackupEncryptionKeyInstallResponse = {
  secret: string;
  secretPath: string;
  projectPath: string;
  output: string;
};

export type BackupEncryptionKeyStatus = {
  configured: boolean;
  readable: boolean;
  keyId?: string;
  path: string;
};

export function useStorageLocations() {
  return useQuery({
    queryKey: ["storage-locations"],
    queryFn: () =>
      api
        .get<StorageLocation[]>("/storage-locations")
        .then((r) => r.data ?? []),
  });
}

export function useCreateStorageLocation() {
  const queryClient = useQueryClient();
  const { t } = useTranslation("settings");

  return useMutation({
    mutationFn: (body: StorageLocationInput) =>
      api.post<StorageLocation>("/storage-locations", body).then(assertSuccess),
    meta: { success: () => t("toast.storageLocationCreated") },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["storage-locations"] });
    },
  });
}

export function useUpdateStorageLocation() {
  const queryClient = useQueryClient();
  const { t } = useTranslation("settings");

  return useMutation({
    mutationFn: ({
      id,
      ...body
    }: { id: string } & Partial<StorageLocationInput>) =>
      api
        .put<StorageLocation>(`/storage-locations/${id}`, body)
        .then(assertSuccess),
    meta: { success: () => t("toast.storageLocationUpdated") },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["storage-locations"] });
    },
  });
}

export function useDeleteStorageLocation() {
  const queryClient = useQueryClient();
  const { t } = useTranslation("settings");

  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/storage-locations/${id}`).then(assertSuccess),
    meta: { success: () => t("toast.storageLocationDeleted") },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["storage-locations"] });
    },
  });
}

export function useAuthorizeStorageLocation() {
  const { t } = useTranslation("settings");

  return useMutation({
    mutationFn: (id: string) =>
      api
        .post<StorageOAuthAuthorizeResponse>(
          `/storage-locations/${id}/oauth/authorize`,
        )
        .then(assertSuccess),
    meta: { error: t("toast.storageConsentFailed") },
  });
}

export function useMigrateStorageLocationBackups() {
  const queryClient = useQueryClient();
  const { t } = useTranslation("settings");

  return useMutation({
    mutationFn: (id: string) =>
      api
        .post<StorageLocationBackupMigrationResult>(
          `/storage-locations/${id}/migrate-container-backups`,
        )
        .then(assertSuccess),
    meta: {
      success: (data: unknown) => {
        const result = data as StorageLocationBackupMigrationResult;
        return t("toast.storageBackupsMigrated", {
          migrated: result.migrated,
          skipped: result.skipped,
          failed: result.failed,
        });
      },
      error: t("toast.storageBackupsMigrationFailed"),
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["container-backup-runs"] });
    },
  });
}

export function useGenerateBackupEncryptionKey() {
  const { t } = useTranslation("settings");

  return useMutation({
    mutationFn: () =>
      api
        .post<BackupEncryptionKeyResponse>("/settings/backup-key/generate")
        .then(assertSuccess),
    meta: { success: () => t("toast.backupKeyGenerated") },
  });
}

export function useBackupEncryptionKeyStatus() {
  return useQuery({
    queryKey: ["backup-encryption-key-status"],
    queryFn: () =>
      api
        .get<BackupEncryptionKeyStatus>("/settings/backup-key/status")
        .then((r) => r.data),
  });
}

export function useInstallBackupEncryptionKey() {
  const queryClient = useQueryClient();
  const { t } = useTranslation("settings");

  return useMutation({
    mutationFn: (key: string) =>
      api
        .post<BackupEncryptionKeyInstallResponse>(
          "/settings/backup-key/install",
          { key },
        )
        .then(assertSuccess),
    meta: { success: () => t("toast.backupKeyInstalled") },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["backup-encryption-key-status"],
      });
    },
  });
}
