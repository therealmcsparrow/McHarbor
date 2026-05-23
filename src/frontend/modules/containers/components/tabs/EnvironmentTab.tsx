// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { ContainerInspect } from "@core/types/docker";
import type { EditFormData, HealthcheckConfig } from "../../types/edit-form";
import {
  EnvironmentConfigSection,
  EnvironmentConsoleSection,
  EnvironmentHealthcheckSection,
  EnvironmentImageSection,
  EnvironmentLogOptionsSection,
  EnvironmentVariablesSection,
} from "./EnvironmentSections";

type EnvironmentTabProps = {
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: <K extends keyof EditFormData>(
    field: K,
    value: EditFormData[K],
  ) => void;
};

const DEFAULT_HEALTHCHECK: HealthcheckConfig = {
  enabled: false,
  command: "",
  interval: 30,
  timeout: 30,
  retries: 3,
  startPeriod: 0,
};

export function EnvironmentTab({
  container,
  editing,
  editData,
  onFieldChange,
}: EnvironmentTabProps) {
  const { t } = useTranslation("containers");

  const envEntries = useMemo(() => {
    const envList = editing
      ? (editData?.env ?? [])
      : (container.Config?.Env ?? []);
    return envList.map((entry) => {
      const idx = entry.indexOf("=");
      return {
        key: idx > -1 ? entry.slice(0, idx) : entry,
        value: idx > -1 ? entry.slice(idx + 1) : "",
      };
    });
  }, [container.Config?.Env, editData?.env, editing]);

  const logOptionEntries = useMemo(() => {
    const options = editing
      ? (editData?.logOptions ?? {})
      : (container.HostConfig?.LogConfig?.Config ?? {});
    return Object.entries(options).map(([key, value]) => ({ key, value }));
  }, [container.HostConfig?.LogConfig?.Config, editData?.logOptions, editing]);

  const handleEnvChange = useCallback(
    (entries: Array<{ key: string; value: string }>) => {
      onFieldChange(
        "env",
        entries.map((entry) => `${entry.key}=${entry.value}`),
      );
    },
    [onFieldChange],
  );

  const handleLogOptionChange = useCallback(
    (entries: Array<{ key: string; value: string }>) => {
      onFieldChange(
        "logOptions",
        Object.fromEntries(entries.map((entry) => [entry.key, entry.value])),
      );
    },
    [onFieldChange],
  );

  const handleHealthcheckChange = useCallback(
    <K extends keyof HealthcheckConfig>(
      field: K,
      value: HealthcheckConfig[K],
    ) => {
      onFieldChange("healthcheck", {
        ...(editData?.healthcheck ?? DEFAULT_HEALTHCHECK),
        [field]: value,
      });
    },
    [editData?.healthcheck, onFieldChange],
  );

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      {editing && (
        <EnvironmentImageSection
          t={t}
          image={editData?.image ?? ""}
          onFieldChange={onFieldChange}
        />
      )}
      <EnvironmentVariablesSection
        t={t}
        editing={editing}
        envEntries={envEntries}
        onEnvChange={handleEnvChange}
      />
      <EnvironmentConfigSection
        t={t}
        container={container}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
      />
      <EnvironmentConsoleSection
        t={t}
        container={container}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
      />
      <EnvironmentLogOptionsSection
        t={t}
        editing={editing}
        logOptionEntries={logOptionEntries}
        onLogOptionChange={handleLogOptionChange}
      />
      {editing && (
        <EnvironmentHealthcheckSection
          t={t}
          healthcheck={editData?.healthcheck ?? DEFAULT_HEALTHCHECK}
          onHealthcheckChange={handleHealthcheckChange}
        />
      )}
    </div>
  );
}
