// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconPlus, IconTrash } from "@tabler/icons-react";
import type { ContainerInspect } from "@core/types/docker";
import { Button } from "@resources/components/ui/Button";
import { InfoRow } from "@resources/components/ui/InfoRow";
import type { DeviceMappingEntry, EditFormData } from "../../types/edit-form";
import { EditInput } from "./EditFieldControls";

type Translator = (key: string) => string;
type ChangeHandler = <K extends keyof EditFormData>(
  field: K,
  value: EditFormData[K],
) => void;

type DevicesSectionProps = {
  t: Translator;
  hc: ContainerInspect["HostConfig"];
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
};

const DEFAULT_DEVICE: DeviceMappingEntry = {
  pathOnHost: "",
  pathInContainer: "",
  cgroupPermissions: "rwm",
};

function updateDevice(
  devices: DeviceMappingEntry[],
  index: number,
  patch: Partial<DeviceMappingEntry>,
) {
  return devices.map((device, currentIndex) =>
    currentIndex === index ? { ...device, ...patch } : device,
  );
}

export function DevicesSection({
  t,
  hc,
  editing,
  editData,
  onFieldChange,
}: DevicesSectionProps) {
  const devices = editData?.devices ?? [];
  const inspectedDevices = hc?.Devices ?? [];

  if (!editing && inspectedDevices.length === 0) {
    return null;
  }

  return (
    <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
      <div className="mb-4 flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          {t("overview.devices")}
        </h3>
        {editing && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => onFieldChange("devices", [...devices, DEFAULT_DEVICE])}
          >
            <IconPlus className="size-4" />
            {t("resources.addDevice")}
          </Button>
        )}
      </div>

      {editing ? (
        <div className="space-y-4">
          {devices.length === 0 ? (
            <div className="rounded-md border border-dashed border-border bg-muted/30 p-4 text-xs text-muted-foreground">
              {t("resources.noDevices")}
            </div>
          ) : (
            devices.map((device, index) => (
              <div
                key={`device-${index}-${device.pathOnHost}`}
                className="rounded-md border border-border bg-muted/30 p-3"
              >
                <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_1fr_120px_auto]">
                  <EditInput
                    label={t("resources.deviceHostPath")}
                    value={device.pathOnHost}
                    onChange={(value) =>
                      onFieldChange(
                        "devices",
                        updateDevice(devices, index, { pathOnHost: value }),
                      )
                    }
                    placeholder="/dev/ttyUSB0"
                  />
                  <EditInput
                    label={t("resources.deviceContainerPath")}
                    value={device.pathInContainer}
                    onChange={(value) =>
                      onFieldChange(
                        "devices",
                        updateDevice(devices, index, { pathInContainer: value }),
                      )
                    }
                    placeholder="/dev/ttyUSB0"
                  />
                  <EditInput
                    label={t("resources.devicePermissions")}
                    value={device.cgroupPermissions}
                    onChange={(value) =>
                      onFieldChange(
                        "devices",
                        updateDevice(devices, index, { cgroupPermissions: value }),
                      )
                    }
                    placeholder="rwm"
                  />
                  <div className="flex items-end">
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={t("resources.removeDevice")}
                      onClick={() =>
                        onFieldChange(
                          "devices",
                          devices.filter((_, currentIndex) => currentIndex !== index),
                        )
                      }
                    >
                      <IconTrash className="size-4" />
                    </Button>
                  </div>
                </div>
              </div>
            ))
          )}
          <p className="text-[10px] text-muted-foreground/70">
            {t("resources.devicesHelp")}
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-x-8 md:grid-cols-2">
          {inspectedDevices.map((device) => (
            <InfoRow
              key={`${device.PathOnHost}-${device.PathInContainer}`}
              label={device.PathOnHost}
            >
              <span className="font-mono">
                {device.PathInContainer}
                {device.CgroupPermissions ? `:${device.CgroupPermissions}` : ""}
              </span>
            </InfoRow>
          ))}
        </div>
      )}
    </div>
  );
}
