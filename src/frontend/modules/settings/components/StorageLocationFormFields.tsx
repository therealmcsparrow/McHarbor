// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { Input } from "@resources/components/ui/Input";
import { Label } from "@resources/components/ui/Label";
import { Select } from "@resources/components/ui/Select";
import { Switch } from "@resources/components/ui/Switch";
import { Textarea } from "@resources/components/ui/Textarea";
import type { StorageLocationInput } from "../hooks/useStorageLocations";

type StorageLocationFormFieldsProps = {
  data: StorageLocationInput;
  isEdit?: boolean;
  onChange: (patch: Partial<StorageLocationInput>) => void;
};

type FieldProps = {
  id: string;
  label: string;
  value: string | number | undefined;
  onChange: (value: string) => void;
  type?: string;
  placeholder?: string;
  helper?: string;
};

type TextareaFieldProps = {
  id: string;
  label: string;
  value: string | undefined;
  onChange: (value: string) => void;
  placeholder?: string;
  helper?: string;
};

function Field({
  id,
  label,
  value,
  onChange,
  type = "text",
  placeholder,
  helper,
}: FieldProps) {
  return (
    <div>
      <Label htmlFor={id} className="mb-1 text-xs text-muted-foreground">
        {label}
      </Label>
      <Input
        id={id}
        type={type}
        value={value ?? ""}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        variant="outline"
      />
      {helper && <p className="mt-1 text-xs text-muted-foreground">{helper}</p>}
    </div>
  );
}

function TextareaField({
  id,
  label,
  value,
  onChange,
  placeholder,
  helper,
}: TextareaFieldProps) {
  return (
    <div className="sm:col-span-2">
      <Label htmlFor={id} className="mb-1 text-xs text-muted-foreground">
        {label}
      </Label>
      <Textarea
        id={id}
        value={value ?? ""}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        variant="outline"
        className="min-h-[120px] font-mono text-xs"
      />
      {helper && <p className="mt-1 text-xs text-muted-foreground">{helper}</p>}
    </div>
  );
}

function InfoPanel({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">
      {children}
    </div>
  );
}

export function StorageLocationFormFields({
  data,
  isEdit,
  onChange,
}: StorageLocationFormFieldsProps) {
  const { t } = useTranslation("settings");
  const secretPlaceholder = isEdit ? t("storage.secretHint") : undefined;

  if (data.locationType === "local") {
    return (
      <div className="space-y-4">
        <InfoPanel>{t("storage.localPathHint")}</InfoPanel>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field
            id="storage-local-path"
            label={t("storage.localPathLabel")}
            value={data.basePath}
            onChange={(basePath) => onChange({ basePath })}
            placeholder="/mnt/backup"
            helper={t("storage.localPathHelper")}
          />
        </div>
      </div>
    );
  }

  if (data.locationType === "ftp" || data.locationType === "ftps") {
    const protocolValue =
      data.locationType === "ftps" && data.tlsMode === "implicit"
        ? "ftps_implicit"
        : data.locationType === "ftps"
          ? "ftps_explicit"
          : "ftp";

    function updateProtocol(value: string) {
      if (value === "ftp") {
        onChange({ locationType: "ftp", tlsMode: undefined, port: 21 });
        return;
      }
      if (value === "ftps_implicit") {
        onChange({ locationType: "ftps", tlsMode: "implicit", port: 990 });
        return;
      }
      onChange({ locationType: "ftps", tlsMode: "explicit", port: 21 });
    }

    return (
      <div className="space-y-4">
        <InfoPanel>
          {data.locationType === "ftp"
            ? t("storage.ftpSecurityHint")
            : t("storage.ftpsSecurityHint")}
        </InfoPanel>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <Label className="mb-1 text-xs text-muted-foreground">
              {t("storage.protocolLabel")}
            </Label>
            <Select
              value={protocolValue}
              onChange={updateProtocol}
              options={[
                { value: "ftp", label: t("storage.protocolFtp") },
                {
                  value: "ftps_explicit",
                  label: t("storage.protocolFtpsExplicit"),
                },
                {
                  value: "ftps_implicit",
                  label: t("storage.protocolFtpsImplicit"),
                },
              ]}
              searchable={false}
              variant="outline"
              ariaLabel={t("storage.protocolLabel")}
              portalled={false}
            />
          </div>
          <Field
            id="storage-host"
            label={t("storage.hostLabel")}
            value={data.host}
            onChange={(host) => onChange({ host })}
            placeholder="ftp.example.com"
          />
          <Field
            id="storage-port"
            label={t("storage.portLabel")}
            type="number"
            value={data.port ?? ""}
            onChange={(port) => onChange({ port: Number(port) || undefined })}
            placeholder={data.locationType === "ftps" ? "21 / 990" : "21"}
            helper={
              data.locationType === "ftps"
                ? t("storage.ftpsPortHint")
                : t("storage.ftpPortHint")
            }
          />
          <Field
            id="storage-path"
            label={t("storage.basePathLabel")}
            value={data.basePath}
            onChange={(basePath) => onChange({ basePath })}
            placeholder="/backups"
          />
          <Field
            id="storage-username"
            label={t("storage.usernameLabel")}
            value={data.username}
            onChange={(username) => onChange({ username })}
            helper={t("storage.ftpAuthHint")}
          />
          <Field
            id="storage-password"
            label={t("storage.passwordLabel")}
            type="password"
            value={data.password}
            onChange={(password) => onChange({ password })}
            placeholder={secretPlaceholder}
          />
          <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/30 p-3 sm:col-span-2">
            <div>
              <p className="text-sm font-medium text-foreground">
                {t("storage.passiveModeLabel")}
              </p>
              <p className="text-xs text-muted-foreground">
                {t("storage.passiveModeDescription")}
              </p>
            </div>
            <Switch
              checked={data.passiveMode ?? true}
              onCheckedChange={(passiveMode) => onChange({ passiveMode })}
              aria-label={t("storage.passiveModeLabel")}
            />
          </div>
          {data.locationType === "ftps" && (
            <>
              <TextareaField
                id="storage-ca-certificate"
                label={t("storage.caCertificateLabel")}
                value={data.caCertificate}
                onChange={(caCertificate) => onChange({ caCertificate })}
                placeholder={secretPlaceholder}
                helper={t("storage.caCertificateHint")}
              />
              <TextareaField
                id="storage-client-certificate"
                label={t("storage.clientCertificateLabel")}
                value={data.clientCertificate}
                onChange={(clientCertificate) =>
                  onChange({ clientCertificate })
                }
                placeholder={secretPlaceholder}
                helper={t("storage.clientCertificateHint")}
              />
              <TextareaField
                id="storage-client-key"
                label={t("storage.clientKeyLabel")}
                value={data.clientKey}
                onChange={(clientKey) => onChange({ clientKey })}
                placeholder={secretPlaceholder}
                helper={t("storage.clientKeyHint")}
              />
            </>
          )}
        </div>
      </div>
    );
  }

  if (data.locationType === "sftp") {
    const authMethod = data.authMethod ?? "private_key";

    return (
      <div className="space-y-4">
        <InfoPanel>{t("storage.sftpSecurityHint")}</InfoPanel>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <Label className="mb-1 text-xs text-muted-foreground">
              {t("storage.authMethodLabel")}
            </Label>
            <Select
              value={authMethod}
              onChange={(value) =>
                onChange({
                  authMethod: value as StorageLocationInput["authMethod"],
                })
              }
              options={[
                { value: "private_key", label: t("storage.authPrivateKey") },
                { value: "password", label: t("storage.authPassword") },
                {
                  value: "password_private_key",
                  label: t("storage.authPasswordPrivateKey"),
                },
              ]}
              searchable={false}
              variant="outline"
              ariaLabel={t("storage.authMethodLabel")}
              portalled={false}
            />
          </div>
          <Field
            id="storage-host"
            label={t("storage.hostLabel")}
            value={data.host}
            onChange={(host) => onChange({ host })}
            placeholder="sftp.example.com"
          />
          <Field
            id="storage-port"
            label={t("storage.portLabel")}
            type="number"
            value={data.port ?? ""}
            onChange={(port) => onChange({ port: Number(port) || undefined })}
            placeholder="22"
            helper={t("storage.sftpPortHint")}
          />
          <Field
            id="storage-path"
            label={t("storage.basePathLabel")}
            value={data.basePath}
            onChange={(basePath) => onChange({ basePath })}
            placeholder="/backups"
          />
          <Field
            id="storage-username"
            label={t("storage.usernameLabel")}
            value={data.username}
            onChange={(username) => onChange({ username })}
          />
          {(authMethod === "password" ||
            authMethod === "password_private_key") && (
            <Field
              id="storage-password"
              label={t("storage.passwordLabel")}
              type="password"
              value={data.password}
              onChange={(password) => onChange({ password })}
              placeholder={secretPlaceholder}
            />
          )}
          {(authMethod === "private_key" ||
            authMethod === "password_private_key") && (
            <>
              <TextareaField
                id="storage-private-key"
                label={t("storage.privateKeyLabel")}
                value={data.privateKey}
                onChange={(privateKey) => onChange({ privateKey })}
                placeholder={secretPlaceholder}
                helper={t("storage.privateKeyHint")}
              />
              <Field
                id="storage-passphrase"
                label={t("storage.passphraseLabel")}
                type="password"
                value={data.passphrase}
                onChange={(passphrase) => onChange({ passphrase })}
                placeholder={secretPlaceholder}
                helper={t("storage.passphraseHint")}
              />
            </>
          )}
        </div>
      </div>
    );
  }

  if (data.locationType === "samba") {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Field
          id="storage-host"
          label={t("storage.hostLabel")}
          value={data.host}
          onChange={(host) => onChange({ host })}
          placeholder="fileserver.local"
        />
        <Field
          id="storage-share"
          label={t("storage.shareNameLabel")}
          value={data.shareName}
          onChange={(shareName) => onChange({ shareName })}
          placeholder="backups"
        />
        <Field
          id="storage-domain"
          label={t("storage.domainLabel")}
          value={data.domain}
          onChange={(domain) => onChange({ domain })}
        />
        <Field
          id="storage-path"
          label={t("storage.basePathLabel")}
          value={data.basePath}
          onChange={(basePath) => onChange({ basePath })}
          placeholder="/mcharbor"
        />
        <Field
          id="storage-username"
          label={t("storage.usernameLabel")}
          value={data.username}
          onChange={(username) => onChange({ username })}
        />
        <Field
          id="storage-password"
          label={t("storage.passwordLabel")}
          type="password"
          value={data.password}
          onChange={(password) => onChange({ password })}
          placeholder={secretPlaceholder}
        />
      </div>
    );
  }

  if (data.locationType === "aws") {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Field
          id="storage-bucket"
          label={t("storage.bucketLabel")}
          value={data.bucket}
          onChange={(bucket) => onChange({ bucket })}
          placeholder="mcharbor-backups"
        />
        <Field
          id="storage-region"
          label={t("storage.regionLabel")}
          value={data.region}
          onChange={(region) => onChange({ region })}
          placeholder="eu-west-1"
        />
        <Field
          id="storage-endpoint"
          label={t("storage.endpointLabel")}
          value={data.endpoint}
          onChange={(endpoint) => onChange({ endpoint })}
          placeholder="https://s3.amazonaws.com"
        />
        <Field
          id="storage-path"
          label={t("storage.basePathLabel")}
          value={data.basePath}
          onChange={(basePath) => onChange({ basePath })}
          placeholder="mcharbor/"
        />
        <Field
          id="storage-access-key"
          label={t("storage.accessKeyIdLabel")}
          type="password"
          value={data.accessKeyId}
          onChange={(accessKeyId) => onChange({ accessKeyId })}
          placeholder={secretPlaceholder}
        />
        <Field
          id="storage-secret-key"
          label={t("storage.secretAccessKeyLabel")}
          type="password"
          value={data.secretAccessKey}
          onChange={(secretAccessKey) => onChange({ secretAccessKey })}
          placeholder={secretPlaceholder}
        />
      </div>
    );
  }

  if (
    data.locationType === "google_drive" ||
    data.locationType === "onedrive_personal"
  ) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
          {t("storage.consentDescription")}
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field
            id="storage-drive"
            label={t("storage.driveIdLabel")}
            value={data.driveId}
            onChange={(driveId) => onChange({ driveId })}
            helper={t("storage.driveIdConsentHint")}
          />
          <Field
            id="storage-path"
            label={t("storage.basePathLabel")}
            value={data.basePath}
            onChange={(basePath) => onChange({ basePath })}
            placeholder="/McHarbor"
          />
          <Field
            id="storage-client-id"
            label={t("storage.clientIdLabel")}
            type="password"
            value={data.clientId}
            onChange={(clientId) => onChange({ clientId })}
            placeholder={secretPlaceholder}
          />
          <Field
            id="storage-client-secret"
            label={t("storage.clientSecretLabel")}
            type="password"
            value={data.clientSecret}
            onChange={(clientSecret) => onChange({ clientSecret })}
            placeholder={secretPlaceholder}
          />
        </div>
      </div>
    );
  }

  if (data.locationType === "onedrive_business") {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
          {t("storage.microsoftBusinessConsentDescription")}
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field
            id="storage-tenant"
            label={t("storage.tenantIdLabel")}
            value={data.tenantId}
            onChange={(tenantId) => onChange({ tenantId })}
            helper={t("storage.tenantIdConsentHint")}
          />
          <Field
            id="storage-drive"
            label={t("storage.driveIdLabel")}
            value={data.driveId}
            onChange={(driveId) => onChange({ driveId })}
            helper={t("storage.driveIdConsentHint")}
          />
          <Field
            id="storage-path"
            label={t("storage.basePathLabel")}
            value={data.basePath}
            onChange={(basePath) => onChange({ basePath })}
            placeholder="/McHarbor"
          />
          <Field
            id="storage-client-id"
            label={t("storage.clientIdLabel")}
            type="password"
            value={data.clientId}
            onChange={(clientId) => onChange({ clientId })}
            placeholder={secretPlaceholder}
          />
          <Field
            id="storage-client-secret"
            label={t("storage.clientSecretLabel")}
            type="password"
            value={data.clientSecret}
            onChange={(clientSecret) => onChange({ clientSecret })}
            placeholder={secretPlaceholder}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
        {t("storage.microsoftBusinessConsentDescription")}
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Field
          id="storage-tenant"
          label={t("storage.tenantIdLabel")}
          value={data.tenantId}
          onChange={(tenantId) => onChange({ tenantId })}
          helper={t("storage.tenantIdConsentHint")}
        />
        <Field
          id="storage-site"
          label={t("storage.siteUrlLabel")}
          value={data.siteUrl}
          onChange={(siteUrl) => onChange({ siteUrl })}
          placeholder="https://contoso.sharepoint.com/sites/backups"
        />
        <Field
          id="storage-drive"
          label={t("storage.driveIdLabel")}
          value={data.driveId}
          onChange={(driveId) => onChange({ driveId })}
          helper={t("storage.driveIdConsentHint")}
        />
        <Field
          id="storage-path"
          label={t("storage.basePathLabel")}
          value={data.basePath}
          onChange={(basePath) => onChange({ basePath })}
          placeholder="/McHarbor"
        />
        <Field
          id="storage-client-id"
          label={t("storage.clientIdLabel")}
          type="password"
          value={data.clientId}
          onChange={(clientId) => onChange({ clientId })}
          placeholder={secretPlaceholder}
        />
        <Field
          id="storage-client-secret"
          label={t("storage.clientSecretLabel")}
          type="password"
          value={data.clientSecret}
          onChange={(clientSecret) => onChange({ clientSecret })}
          placeholder={secretPlaceholder}
        />
      </div>
    </div>
  );
}
