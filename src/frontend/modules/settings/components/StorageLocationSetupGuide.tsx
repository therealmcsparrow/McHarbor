// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { Input } from "@resources/components/ui/Input";
import type { StorageLocationType } from "../hooks/useStorageLocations";
import { storageProvider } from "./storage-location-options";

type StorageLocationSetupGuideProps = {
  locationType: StorageLocationType;
};

const GUIDE_STEPS: Record<StorageLocationType, string[]> = {
  ftp: ["networkEndpoint", "networkCredentials", "basePath"],
  ftps: ["networkEndpoint", "tlsEndpoint", "networkCredentials", "basePath"],
  sftp: ["networkEndpoint", "sftpAccess", "networkCredentials", "basePath"],
  samba: ["sambaShare", "networkCredentials", "basePath"],
  aws: ["awsBucket", "awsCredentials", "basePath"],
  google_drive: [
    "googleProject",
    "googleApi",
    "googleOauthClient",
    "googleRedirect",
    "googleConsent",
  ],
  onedrive_personal: [
    "microsoftAppPersonal",
    "microsoftRedirect",
    "accountConsent",
  ],
  onedrive_business: [
    "microsoftAppBusiness",
    "microsoftTenant",
    "microsoftRedirect",
    "microsoftGraph",
  ],
  sharepoint: [
    "microsoftAppBusiness",
    "microsoftTenant",
    "microsoftRedirect",
    "sharepointSite",
    "microsoftGraph",
  ],
};

function oauthRedirectUri() {
  if (typeof window === "undefined")
    return "/api/storage-locations/oauth/callback";
  return `${window.location.origin}/api/storage-locations/oauth/callback`;
}

function showsRedirectUri(locationType: StorageLocationType) {
  return (
    locationType === "google_drive" ||
    locationType === "onedrive_personal" ||
    locationType === "onedrive_business" ||
    locationType === "sharepoint"
  );
}

export function StorageLocationSetupGuide({
  locationType,
}: StorageLocationSetupGuideProps) {
  const { t } = useTranslation("settings");
  const provider = storageProvider(locationType);
  const steps = GUIDE_STEPS[locationType];

  return (
    <div className="rounded-lg border border-border bg-muted/30 p-4">
      <div className="flex items-start gap-3">
        <IconInfoCircle className="mt-0.5 size-5 shrink-0 text-primary" />
        <div className="min-w-0 flex-1 space-y-3">
          <div>
            <h3 className="text-sm font-medium text-foreground">
              {t("storage.setupGuideTitle", { provider: t(provider.labelKey) })}
            </h3>
            <p className="mt-1 text-xs text-muted-foreground">
              {t("storage.setupGuideDescription")}
            </p>
          </div>

          <ol className="list-decimal space-y-1.5 pl-4 text-xs text-muted-foreground">
            {steps.map((step) => (
              <li key={step}>{t(`storage.setupGuide.${step}`)}</li>
            ))}
          </ol>

          {showsRedirectUri(locationType) && (
            <div>
              <p className="mb-1 text-xs font-medium text-foreground">
                {t("storage.redirectUriLabel")}
              </p>
              <Input
                value={oauthRedirectUri()}
                readOnly
                variant="outline"
                className="font-mono text-xs"
                aria-label={t("storage.redirectUriLabel")}
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
