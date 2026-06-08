// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconExternalLink, IconHelpCircle } from "@tabler/icons-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@resources/components/ui/Dialog";
import { Button } from "@resources/components/ui/Button";
import { Input } from "@resources/components/ui/Input";
import { Label } from "@resources/components/ui/Label";
import {
  type StorageLocation,
  type StorageLocationInput,
  type StorageLocationType,
  useAuthorizeStorageLocation,
  useCreateStorageLocation,
  useUpdateStorageLocation,
} from "../hooks/useStorageLocations";
import { StorageLocationFormFields } from "./StorageLocationFormFields";
import { StorageLocationSetupGuide } from "./StorageLocationSetupGuide";
import {
  STORAGE_PROVIDER_CHOICES,
  storageProvider,
} from "./storage-location-options";

type StorageLocationDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  location?: StorageLocation | null;
};

const DEFAULT_FORM: StorageLocationInput = {
  name: "",
  locationType: "sftp",
  enabled: true,
  authMethod: "private_key",
  passiveMode: true,
  port: 22,
};

function supportsConsent(locationType: StorageLocationType) {
  return (
    locationType === "google_drive" ||
    locationType === "onedrive_personal" ||
    locationType === "onedrive_business" ||
    locationType === "sharepoint"
  );
}

function formFromLocation(
  location?: StorageLocation | null,
): StorageLocationInput {
  if (!location) return DEFAULT_FORM;
  return {
    name: location.name,
    locationType: location.locationType,
    enabled: location.enabled,
    host: location.host ?? "",
    port: location.port,
    basePath: location.basePath ?? "",
    region: location.region ?? "",
    bucket: location.bucket ?? "",
    endpoint: location.endpoint ?? "",
    tenantId: location.tenantId ?? "",
    siteUrl: location.siteUrl ?? "",
    driveId: location.driveId ?? "",
    shareName: location.shareName ?? "",
    domain: location.domain ?? "",
    username: location.username ?? "",
    authMethod:
      location.authMethod ??
      (location.locationType === "sftp" ? "private_key" : "password"),
    tlsMode: location.tlsMode,
    passiveMode: location.passiveMode,
  };
}

function cleanInput(input: StorageLocationInput): StorageLocationInput {
  return Object.fromEntries(
    Object.entries(input).filter(
      ([, value]) => value !== "" && value !== undefined,
    ),
  ) as StorageLocationInput;
}

export function StorageLocationDialog({
  open,
  onOpenChange,
  location,
}: StorageLocationDialogProps) {
  const { t } = useTranslation("settings");
  const [step, setStep] = useState<"type" | "config">("type");
  const [form, setForm] = useState<StorageLocationInput>(DEFAULT_FORM);
  const [helpOpen, setHelpOpen] = useState(false);
  const createLocation = useCreateStorageLocation();
  const updateLocation = useUpdateStorageLocation();
  const authorizeLocation = useAuthorizeStorageLocation();
  const isEdit = !!location;
  const provider = storageProvider(form.locationType);
  const ProviderIcon = provider.icon;

  useEffect(() => {
    if (open) {
      setForm(formFromLocation(location));
      setStep(location ? "config" : "type");
      setHelpOpen(false);
    }
  }, [location, open]);

  function update(patch: Partial<StorageLocationInput>) {
    setForm((current) => ({ ...current, ...patch }));
  }

  function handleOpenChange(value: boolean) {
    if (!value) {
      setStep("type");
      setForm(DEFAULT_FORM);
      setHelpOpen(false);
    }
    onOpenChange(value);
  }

  function handleSelectType(locationType: StorageLocationType) {
    if (locationType === "ftp") {
      update({
        locationType,
        authMethod: "password",
        passiveMode: true,
        port: 21,
        tlsMode: undefined,
      });
      setStep("config");
      return;
    }
    if (locationType === "sftp") {
      update({
        locationType,
        authMethod: "private_key",
        passiveMode: true,
        port: 22,
        tlsMode: undefined,
      });
      setStep("config");
      return;
    }
    update({ locationType });
    setStep("config");
  }

  function handleBack() {
    setStep("type");
    setHelpOpen(false);
  }

  function connectLocation(id: string) {
    authorizeLocation.mutate(id, {
      onSuccess: (data) => {
        window.location.assign(data.authorizationUrl);
      },
    });
  }

  function handleSave(connectAfterSave = false) {
    const body = cleanInput(form);
    if (isEdit && location) {
      updateLocation.mutate(
        { id: location.id, ...body },
        {
          onSuccess: (updated) => {
            if (connectAfterSave && supportsConsent(updated.locationType)) {
              connectLocation(updated.id);
              return;
            }
            onOpenChange(false);
          },
        },
      );
      return;
    }

    createLocation.mutate(body, {
      onSuccess: (created) => {
        if (connectAfterSave && supportsConsent(created.locationType)) {
          connectLocation(created.id);
          return;
        }
        onOpenChange(false);
      },
    });
  }

  const saving =
    createLocation.isPending ||
    updateLocation.isPending ||
    authorizeLocation.isPending;
  const valid = form.name.trim() !== "" && form.locationType !== undefined;
  const canSaveAndConnect = supportsConsent(form.locationType);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t("storage.editLocation")
              : step === "type"
                ? t("storage.selectType")
                : t("storage.configuration")}
          </DialogTitle>
          <DialogDescription>
            {step === "type"
              ? t("storage.selectTypeDescription")
              : t(provider.descriptionKey)}
          </DialogDescription>
        </DialogHeader>

        <div>
          {step === "type" && (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
              {STORAGE_PROVIDER_CHOICES.map(
                ({ type, icon: Icon, labelKey }) => (
                  <Button
                    key={type}
                    variant="outline"
                    onClick={() => handleSelectType(type)}
                    className="flex h-auto flex-col items-center gap-3 p-4 text-center"
                  >
                    <Icon className="size-8 text-primary" />
                    <span className="text-xs font-medium text-foreground">
                      {t(labelKey)}
                    </span>
                  </Button>
                ),
              )}
            </div>
          )}

          {step === "config" && (
            <div className="space-y-4">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <Label
                    htmlFor="storage-name"
                    className="mb-1 text-xs text-muted-foreground"
                  >
                    {t("storage.nameLabel")}
                  </Label>
                  <Input
                    id="storage-name"
                    value={form.name}
                    onChange={(event) => update({ name: event.target.value })}
                    placeholder={t("storage.namePlaceholder")}
                    variant="outline"
                  />
                </div>
                <div>
                  <Label className="mb-1 text-xs text-muted-foreground">
                    {t("storage.typeLabel")}
                  </Label>
                  <div className="flex min-h-10 items-center gap-2 rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground">
                    <ProviderIcon className="size-4 text-primary" />
                    {t(provider.labelKey)}
                  </div>
                </div>
              </div>

              <StorageLocationFormFields
                data={form}
                isEdit={isEdit}
                onChange={update}
              />

              {helpOpen && (
                <StorageLocationSetupGuide locationType={form.locationType} />
              )}

              <p className="text-xs text-muted-foreground">
                {t("storage.credentialsDescription")}
              </p>
            </div>
          )}
        </div>

        {step === "config" && (
          <DialogFooter className="flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:justify-between">
            <Button
              variant="ghost"
              onClick={() => setHelpOpen((current) => !current)}
              aria-expanded={helpOpen}
            >
              <IconHelpCircle className="size-4" />
              {helpOpen
                ? t("storage.hideSetupGuide")
                : t("storage.showSetupGuide")}
            </Button>
            <div className="flex flex-wrap items-center justify-end gap-2">
              {!isEdit && (
                <Button variant="outline" onClick={handleBack}>
                  {t("common:back", "Back")}
                </Button>
              )}
              <Button variant="outline" onClick={() => handleOpenChange(false)}>
                {t("common:cancel", "Cancel")}
              </Button>
              {canSaveAndConnect && (
                <Button
                  variant="outline"
                  onClick={() => handleSave(true)}
                  disabled={!valid || saving}
                >
                  <IconExternalLink className="size-4" />
                  {saving ? "..." : t("storage.saveAndConnect")}
                </Button>
              )}
              <Button
                onClick={() => handleSave(false)}
                disabled={!valid || saving}
              >
                {saving ? "..." : t("common:save", "Save")}
              </Button>
            </div>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
