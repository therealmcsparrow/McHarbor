// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { InfoRow } from "@resources/components/ui/InfoRow";
import { KeyValueEditor } from "../KeyValueEditor";
import type { ContainerInspect } from "@core/types/docker";
import type { EditFormData, HealthcheckConfig } from "../../types/edit-form";
import { EditInput, ToggleField } from "./EditFieldControls";

type ChangeHandler = <K extends keyof EditFormData>(
  field: K,
  value: EditFormData[K],
) => void;
type Translator = (key: string) => string;

export function EnvironmentImageSection({
  t,
  image,
  onFieldChange,
}: {
  t: Translator;
  image: string;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("environment.image")}
      </h3>
      <EditInput
        label={t("overview.image")}
        value={image}
        onChange={(value) => onFieldChange("image", value)}
        placeholder="nginx:latest"
      />
    </div>
  );
}

export function EnvironmentVariablesSection({
  t,
  editing,
  envEntries,
  onEnvChange,
}: {
  t: Translator;
  editing: boolean;
  envEntries: Array<{ key: string; value: string }>;
  onEnvChange: (entries: Array<{ key: string; value: string }>) => void;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("environment.envVars")}
      </h3>
      {editing ? (
        <KeyValueEditor
          entries={envEntries}
          onChange={onEnvChange}
          keyLabel={t("edit.key")}
          valueLabel={t("edit.value")}
          addLabel={t("edit.addEnvVar")}
        />
      ) : envEntries.length > 0 ? (
        <div className="max-h-64 overflow-y-auto">
          {envEntries.map((entry) => (
            <div
              key={`${entry.key}:${entry.value}`}
              className="flex gap-2 border-b border-border py-1.5 last:border-0"
            >
              <span className="shrink-0 font-mono text-xs font-medium text-foreground">
                {entry.key}
              </span>
              <span className="truncate font-mono text-xs text-muted-foreground">
                {entry.value}
              </span>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t("overview.noEnvVars")}
        </p>
      )}
    </div>
  );
}

export function EnvironmentConfigSection({
  t,
  container,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("environment.config")}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <EditInput
            label={t("environment.user")}
            value={editData?.user ?? ""}
            onChange={(value) => onFieldChange("user", value)}
            placeholder="user:group"
          />
          <EditInput
            label={t("environment.hostname")}
            value={editData?.hostname ?? ""}
            onChange={(value) => onFieldChange("hostname", value)}
          />
          <EditInput
            label={t("environment.domainname")}
            value={editData?.domainname ?? ""}
            onChange={(value) => onFieldChange("domainname", value)}
          />
          <EditInput
            label={t("environment.workingDir")}
            value={editData?.workingDir ?? ""}
            onChange={(value) => onFieldChange("workingDir", value)}
          />
          <EditInput
            label={t("environment.command")}
            value={editData?.cmd?.join(" ") ?? ""}
            onChange={(value) =>
              onFieldChange("cmd", value ? value.split(" ") : [])
            }
          />
          <EditInput
            label={t("environment.entrypoint")}
            value={editData?.entrypoint?.join(" ") ?? ""}
            onChange={(value) =>
              onFieldChange("entrypoint", value ? value.split(" ") : [])
            }
          />
          <EditInput
            label={t("environment.stopSignal")}
            value={editData?.stopSignal ?? ""}
            onChange={(value) => onFieldChange("stopSignal", value)}
            placeholder="SIGTERM"
          />
        </div>
      ) : (
        <>
          <InfoRow label={t("environment.user")}>
            {container.Config?.User || "-"}
          </InfoRow>
          <InfoRow label={t("environment.hostname")}>
            {container.Config?.Hostname ?? "-"}
          </InfoRow>
          <InfoRow label={t("environment.domainname")}>
            {container.Config?.Domainname || "-"}
          </InfoRow>
          <InfoRow label={t("environment.workingDir")}>
            {container.Config?.WorkingDir || "/"}
          </InfoRow>
          <InfoRow label={t("environment.command")}>
            <span className="font-mono text-xs">
              {container.Config?.Cmd?.join(" ") ?? "-"}
            </span>
          </InfoRow>
          <InfoRow label={t("environment.entrypoint")}>
            <span className="font-mono text-xs">
              {container.Config?.Entrypoint?.join(" ") ?? "-"}
            </span>
          </InfoRow>
          <InfoRow label={t("environment.stopSignal")}>
            {container.Config?.StopSignal || "SIGTERM"}
          </InfoRow>
        </>
      )}
    </div>
  );
}

export function EnvironmentConsoleSection({
  t,
  container,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("environment.console")}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <ToggleField
            label={t("environment.tty")}
            checked={editData?.tty ?? false}
            onChange={(value) => onFieldChange("tty", value)}
          />
          <ToggleField
            label={t("environment.openStdin")}
            checked={editData?.openStdin ?? false}
            onChange={(value) => onFieldChange("openStdin", value)}
          />
          <EditInput
            label={t("environment.logDriver")}
            value={editData?.logDriver ?? ""}
            onChange={(value) => onFieldChange("logDriver", value)}
            placeholder="json-file"
          />
        </div>
      ) : (
        <>
          <InfoRow label={t("environment.tty")}>
            {container.Config?.Tty
              ? t("common:labels.yes")
              : t("common:labels.no")}
          </InfoRow>
          <InfoRow label={t("environment.openStdin")}>
            {container.Config?.OpenStdin
              ? t("common:labels.yes")
              : t("common:labels.no")}
          </InfoRow>
          <InfoRow label={t("environment.logDriver")}>
            {container.HostConfig?.LogConfig?.Type ?? "json-file"}
          </InfoRow>
        </>
      )}
    </div>
  );
}

export function EnvironmentLogOptionsSection({
  t,
  editing,
  logOptionEntries,
  onLogOptionChange,
}: {
  t: Translator;
  editing: boolean;
  logOptionEntries: Array<{ key: string; value: string }>;
  onLogOptionChange: (entries: Array<{ key: string; value: string }>) => void;
}) {
  if (!editing && logOptionEntries.length === 0) {
    return null;
  }

  return (
    <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("environment.logOptions")}
      </h3>
      {editing ? (
        <KeyValueEditor
          entries={logOptionEntries}
          onChange={onLogOptionChange}
          keyLabel={t("edit.key")}
          valueLabel={t("edit.value")}
          addLabel={t("edit.addLogOption")}
        />
      ) : (
        <div className="max-h-48 overflow-y-auto">
          {logOptionEntries.map(({ key, value }) => (
            <div
              key={key}
              className="flex gap-2 border-b border-border py-1.5 last:border-0"
            >
              <span className="shrink-0 font-mono text-xs font-medium text-foreground">
                {key}
              </span>
              <span className="truncate font-mono text-xs text-muted-foreground">
                {value}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function EnvironmentHealthcheckSection({
  t,
  healthcheck,
  onHealthcheckChange,
}: {
  t: Translator;
  healthcheck: HealthcheckConfig;
  onHealthcheckChange: <K extends keyof HealthcheckConfig>(
    field: K,
    value: HealthcheckConfig[K],
  ) => void;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("environment.healthcheck")}
      </h3>
      <div className="space-y-3">
        <ToggleField
          label={t("environment.healthEnabled")}
          checked={healthcheck.enabled}
          onChange={(value) => onHealthcheckChange("enabled", value)}
        />
        {healthcheck.enabled && (
          <>
            <EditInput
              label={t("environment.healthCommand")}
              value={healthcheck.command}
              onChange={(value) => onHealthcheckChange("command", value)}
              placeholder="curl -f http://localhost/ || exit 1"
            />
            <div className="grid grid-cols-2 gap-3">
              <EditInput
                label={t("environment.healthInterval")}
                type="number"
                value={healthcheck.interval}
                onChange={(value) =>
                  onHealthcheckChange("interval", parseInt(value, 10) || 30)
                }
              />
              <EditInput
                label={t("environment.healthTimeout")}
                type="number"
                value={healthcheck.timeout}
                onChange={(value) =>
                  onHealthcheckChange("timeout", parseInt(value, 10) || 30)
                }
              />
              <EditInput
                label={t("environment.healthRetries")}
                type="number"
                value={healthcheck.retries}
                onChange={(value) =>
                  onHealthcheckChange("retries", parseInt(value, 10) || 3)
                }
              />
              <EditInput
                label={t("environment.healthStartPeriod")}
                type="number"
                value={healthcheck.startPeriod}
                onChange={(value) =>
                  onHealthcheckChange("startPeriod", parseInt(value, 10) || 0)
                }
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}
