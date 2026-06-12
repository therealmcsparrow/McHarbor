// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconBrandAws,
  IconBrandGoogleDrive,
  IconBrandOffice,
  IconCloud,
  IconFolder,
  IconFolderShare,
  IconServer,
} from "@tabler/icons-react";
import type { TablerIcon } from "@tabler/icons-react";
import type { StorageLocationType } from "../hooks/useStorageLocations";

export type StorageProviderOption = {
  type: StorageLocationType;
  labelKey: string;
  descriptionKey: string;
  icon: TablerIcon;
};

const DEFAULT_STORAGE_PROVIDER: StorageProviderOption = {
  type: "sftp",
  labelKey: "storage.typeSftp",
  descriptionKey: "storage.typeSftpDescription",
  icon: IconServer,
};

export const STORAGE_PROVIDER_OPTIONS: StorageProviderOption[] = [
  {
    type: "local",
    labelKey: "storage.typeLocal",
    descriptionKey: "storage.typeLocalDescription",
    icon: IconFolder,
  },
  {
    type: "ftp",
    labelKey: "storage.typeFtp",
    descriptionKey: "storage.typeFtpDescription",
    icon: IconServer,
  },
  {
    type: "ftps",
    labelKey: "storage.typeFtps",
    descriptionKey: "storage.typeFtpsDescription",
    icon: IconServer,
  },
  {
    type: "sftp",
    labelKey: "storage.typeSftp",
    descriptionKey: "storage.typeSftpDescription",
    icon: IconServer,
  },
  {
    type: "samba",
    labelKey: "storage.typeSamba",
    descriptionKey: "storage.typeSambaDescription",
    icon: IconFolderShare,
  },
  {
    type: "aws",
    labelKey: "storage.typeAws",
    descriptionKey: "storage.typeAwsDescription",
    icon: IconBrandAws,
  },
  {
    type: "google_drive",
    labelKey: "storage.typeGoogleDrive",
    descriptionKey: "storage.typeGoogleDriveDescription",
    icon: IconBrandGoogleDrive,
  },
  {
    type: "onedrive_personal",
    labelKey: "storage.typeOneDrivePersonal",
    descriptionKey: "storage.typeOneDrivePersonalDescription",
    icon: IconCloud,
  },
  {
    type: "onedrive_business",
    labelKey: "storage.typeOneDriveBusiness",
    descriptionKey: "storage.typeOneDriveBusinessDescription",
    icon: IconBrandOffice,
  },
  {
    type: "sharepoint",
    labelKey: "storage.typeSharePoint",
    descriptionKey: "storage.typeSharePointDescription",
    icon: IconBrandOffice,
  },
];

export const STORAGE_PROVIDER_CHOICES: StorageProviderOption[] =
  STORAGE_PROVIDER_OPTIONS.map((option) =>
    option.type === "ftp"
      ? {
          ...option,
          labelKey: "storage.typeFtpFtps",
          descriptionKey: "storage.typeFtpFtpsDescription",
        }
      : option,
  ).filter((option) => option.type !== "ftps");

export function storageProvider(type: StorageLocationType) {
  return (
    STORAGE_PROVIDER_OPTIONS.find((option) => option.type === type) ??
    DEFAULT_STORAGE_PROVIDER
  );
}
