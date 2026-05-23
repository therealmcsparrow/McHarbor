// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Badge } from "@resources/components/ui/Badge";
import { InfoRow } from "@resources/components/ui/InfoRow";
import { formatBytes } from "@resources/utils/format";
import type { ContainerInspect } from "@core/types/docker";
import type { EditFormData } from "../../types/edit-form";
import { EditInput, ToggleField } from "./EditFieldControls";

type Translator = (key: string) => string;
type ChangeHandler = <K extends keyof EditFormData>(
  field: K,
  value: EditFormData[K],
) => void;
type DeviceRequest = NonNullable<
  NonNullable<ContainerInspect["HostConfig"]>["DeviceRequests"]
>[number];

export const RESTART_POLICIES = [
  "no",
  "always",
  "unless-stopped",
  "on-failure",
];

export function splitCsv(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

export function RestartPolicySection({
  t,
  hc,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  hc: ContainerInspect["HostConfig"];
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("resources.restartPolicy")}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <div>
            <label className="text-xs font-medium text-muted-foreground">
              {t("resources.policy")}
            </label>
            <select
              value={editData?.restartPolicyName ?? "no"}
              onChange={(e) =>
                onFieldChange("restartPolicyName", e.target.value)
              }
              className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
            >
              {RESTART_POLICIES.map((policy) => (
                <option key={policy} value={policy}>
                  {policy}
                </option>
              ))}
            </select>
          </div>
          {editData?.restartPolicyName === "on-failure" && (
            <EditInput
              label={t("resources.maxRetries")}
              type="number"
              value={editData?.restartPolicyMaxRetry ?? 0}
              onChange={(value) =>
                onFieldChange("restartPolicyMaxRetry", parseInt(value, 10) || 0)
              }
            />
          )}
        </div>
      ) : (
        <>
          <InfoRow label={t("resources.policy")}>
            {hc?.RestartPolicy?.Name ?? "no"}
          </InfoRow>
          {hc?.RestartPolicy?.Name === "on-failure" && (
            <InfoRow label={t("resources.maxRetries")}>
              {hc?.RestartPolicy?.MaximumRetryCount ?? 0}
            </InfoRow>
          )}
        </>
      )}
    </div>
  );
}

export function MemorySection({
  t,
  hc,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  hc: ContainerInspect["HostConfig"];
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("resources.memory")}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <EditInput
            label={t("resources.memoryLimit")}
            type="number"
            value={editData?.memory ?? 0}
            onChange={(value) =>
              onFieldChange("memory", parseInt(value, 10) || 0)
            }
            suffix="bytes"
          />
          <EditInput
            label={t("resources.memorySwap")}
            type="number"
            value={editData?.memorySwap ?? 0}
            onChange={(value) =>
              onFieldChange("memorySwap", parseInt(value, 10) || 0)
            }
            suffix="bytes"
          />
          <EditInput
            label={t("resources.memoryReservation")}
            type="number"
            value={editData?.memoryReservation ?? 0}
            onChange={(value) =>
              onFieldChange("memoryReservation", parseInt(value, 10) || 0)
            }
            suffix="bytes"
          />
        </div>
      ) : (
        <>
          <InfoRow label={t("resources.memoryLimit")}>
            {hc?.Memory ? formatBytes(hc.Memory) : t("resources.unlimited")}
          </InfoRow>
          <InfoRow label={t("resources.memorySwap")}>
            {hc?.MemorySwap && hc.MemorySwap > 0
              ? formatBytes(hc.MemorySwap)
              : t("resources.unlimited")}
          </InfoRow>
          <InfoRow label={t("resources.memoryReservation")}>
            {hc?.MemoryReservation ? formatBytes(hc.MemoryReservation) : "-"}
          </InfoRow>
        </>
      )}
    </div>
  );
}

export function CpuSection({
  t,
  hc,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  hc: ContainerInspect["HostConfig"];
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("resources.cpu")}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <EditInput
            label={t("resources.nanoCpus")}
            type="number"
            value={editData?.nanoCpus ?? 0}
            onChange={(value) =>
              onFieldChange("nanoCpus", parseInt(value, 10) || 0)
            }
          />
          <EditInput
            label={t("resources.cpuShares")}
            type="number"
            value={editData?.cpuShares ?? 0}
            onChange={(value) =>
              onFieldChange("cpuShares", parseInt(value, 10) || 0)
            }
          />
          <EditInput
            label={t("resources.cpuPeriod")}
            type="number"
            value={editData?.cpuPeriod ?? 0}
            onChange={(value) =>
              onFieldChange("cpuPeriod", parseInt(value, 10) || 0)
            }
            suffix="us"
          />
          <EditInput
            label={t("resources.cpuQuota")}
            type="number"
            value={editData?.cpuQuota ?? 0}
            onChange={(value) =>
              onFieldChange("cpuQuota", parseInt(value, 10) || 0)
            }
            suffix="us"
          />
          <EditInput
            label={t("resources.cpusetCpus")}
            value={editData?.cpusetCpus ?? ""}
            onChange={(value) => onFieldChange("cpusetCpus", value)}
            placeholder="0-3"
          />
        </div>
      ) : (
        <>
          <InfoRow label={t("resources.nanoCpus")}>
            {hc?.NanoCpus
              ? `${(hc.NanoCpus / 1e9).toFixed(2)} cores`
              : t("resources.unlimited")}
          </InfoRow>
          <InfoRow label={t("resources.cpuShares")}>
            {hc?.CpuShares || "-"}
          </InfoRow>
          <InfoRow label={t("resources.cpuPeriod")}>
            {hc?.CpuPeriod ? `${hc.CpuPeriod} us` : "-"}
          </InfoRow>
          <InfoRow label={t("resources.cpuQuota")}>
            {hc?.CpuQuota ? `${hc.CpuQuota} us` : "-"}
          </InfoRow>
          <InfoRow label={t("resources.cpusetCpus")}>
            {hc?.CpusetCpus || "-"}
          </InfoRow>
        </>
      )}
    </div>
  );
}

export function BlockIoSection({
  t,
  hc,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  hc: ContainerInspect["HostConfig"];
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("resources.blockIO")}
      </h3>
      {editing ? (
        <EditInput
          label={t("resources.blkioWeight")}
          type="number"
          value={editData?.blkioWeight ?? 0}
          onChange={(value) =>
            onFieldChange("blkioWeight", parseInt(value, 10) || 0)
          }
          placeholder="0-1000"
        />
      ) : (
        <InfoRow label={t("resources.blkioWeight")}>
          {hc?.BlkioWeight || "-"}
        </InfoRow>
      )}
    </div>
  );
}

export function RuntimeOptionsSection({
  t,
  hc,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  hc: ContainerInspect["HostConfig"];
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("resources.runtimeOptions")}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <ToggleField
            label={t("resources.autoRemove")}
            checked={editData?.autoRemove ?? false}
            onChange={(value) => onFieldChange("autoRemove", value)}
          />
          <ToggleField
            label={t("resources.init")}
            checked={editData?.init ?? false}
            onChange={(value) => onFieldChange("init", value)}
          />
          <ToggleField
            label={t("resources.oomKillDisable")}
            checked={editData?.oomKillDisable ?? false}
            onChange={(value) => onFieldChange("oomKillDisable", value)}
          />
          <EditInput
            label={t("resources.pidsLimit")}
            type="number"
            value={editData?.pidsLimit ?? 0}
            onChange={(value) =>
              onFieldChange("pidsLimit", parseInt(value, 10) || 0)
            }
            placeholder="0 = unlimited"
          />
        </div>
      ) : (
        <>
          <InfoRow label={t("resources.autoRemove")}>
            {hc?.AutoRemove ? (
              <Badge variant="warning">{t("common:labels.yes")}</Badge>
            ) : (
              t("common:labels.no")
            )}
          </InfoRow>
          <InfoRow label={t("resources.init")}>
            {hc?.Init ? t("common:labels.yes") : t("common:labels.no")}
          </InfoRow>
          <InfoRow label={t("resources.oomKillDisable")}>
            {hc?.OomKillDisable ? (
              <Badge variant="destructive">{t("common:labels.yes")}</Badge>
            ) : (
              t("common:labels.no")
            )}
          </InfoRow>
          <InfoRow label={t("resources.pidsLimit")}>
            {hc?.PidsLimit && hc.PidsLimit > 0
              ? hc.PidsLimit
              : t("resources.unlimited")}
          </InfoRow>
        </>
      )}
    </div>
  );
}

export function GpuSection({
  t,
  editing,
  editData,
  gpuRequests,
  onFieldChange,
}: {
  t: Translator;
  editing: boolean;
  editData: EditFormData | null;
  gpuRequests: DeviceRequest[];
  onFieldChange: ChangeHandler;
}) {
  if (!editing && gpuRequests.length === 0) {
    return null;
  }

  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t("overview.gpu")}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <ToggleField
            label={t("resources.enableGpu")}
            checked={editData?.gpuEnabled ?? false}
            onChange={(value) => onFieldChange("gpuEnabled", value)}
          />
          {editData?.gpuEnabled && (
            <>
              <EditInput
                label={t("overview.gpuDriver")}
                value={editData.gpuDriver}
                onChange={(value) => onFieldChange("gpuDriver", value)}
                placeholder="nvidia"
              />
              <EditInput
                label={t("overview.gpuCount")}
                type="number"
                value={editData.gpuCount}
                onChange={(value) =>
                  onFieldChange("gpuCount", parseInt(value, 10) || 0)
                }
              />
              <p className="text-[10px] text-muted-foreground/70">
                {t("resources.gpuCountHelp")}
              </p>
              <EditInput
                label={t("overview.gpuDeviceIDs")}
                value={editData.gpuDeviceIds.join(", ")}
                onChange={(value) =>
                  onFieldChange("gpuDeviceIds", splitCsv(value))
                }
                placeholder="0,1"
              />
              <p className="text-[10px] text-muted-foreground/70">
                {t("resources.gpuDeviceIdsHelp")}
              </p>
              <EditInput
                label={t("overview.gpuCapabilities")}
                value={editData.gpuCapabilities.join(", ")}
                onChange={(value) =>
                  onFieldChange("gpuCapabilities", splitCsv(value))
                }
                placeholder="gpu"
              />
              <p className="text-[10px] text-muted-foreground/70">
                {t("resources.gpuCapabilitiesHelp")}
              </p>
            </>
          )}
        </div>
      ) : (
        gpuRequests.map((request, index) => (
          <div key={`dr-${request.Driver}-${index}`} className="space-y-1">
            <InfoRow label={t("overview.gpuDriver")}>
              {request.Driver || "-"}
            </InfoRow>
            <InfoRow label={t("overview.gpuCount")}>
              {request.Count === -1
                ? t("common:labels.all")
                : String(request.Count)}
            </InfoRow>
            {request.DeviceIDs && request.DeviceIDs.length > 0 && (
              <InfoRow label={t("overview.gpuDeviceIDs")}>
                {request.DeviceIDs.join(", ")}
              </InfoRow>
            )}
            {request.Capabilities && request.Capabilities.length > 0 && (
              <InfoRow label={t("overview.gpuCapabilities")}>
                {request.Capabilities.flat().join(", ")}
              </InfoRow>
            )}
          </div>
        ))
      )}
    </div>
  );
}
