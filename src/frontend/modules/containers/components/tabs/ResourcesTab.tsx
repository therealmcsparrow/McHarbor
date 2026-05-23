// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import type { ContainerInspect } from "@core/types/docker";
import type { EditFormData } from "../../types/edit-form";
import {
  BlockIoSection,
  CpuSection,
  GpuSection,
  MemorySection,
  RestartPolicySection,
  RuntimeOptionsSection,
} from "./ResourcesSections";

type ResourcesTabProps = {
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: <K extends keyof EditFormData>(
    field: K,
    value: EditFormData[K],
  ) => void;
};

export function ResourcesTab({
  container,
  editing,
  editData,
  onFieldChange,
}: ResourcesTabProps) {
  const { t } = useTranslation("containers");
  const hc = container.HostConfig;
  const gpuRequests = editing
    ? editData?.gpuEnabled
      ? [
          {
            Driver: editData.gpuDriver,
            Count: editData.gpuDeviceIds.length > 0 ? 0 : editData.gpuCount,
            DeviceIDs: editData.gpuDeviceIds,
            Capabilities: [
              editData.gpuCapabilities.length > 0
                ? editData.gpuCapabilities
                : ["gpu"],
            ],
            Options: null,
          },
        ]
      : []
    : (hc?.DeviceRequests ?? []);

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <RestartPolicySection
        t={t}
        hc={hc}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
      />
      <MemorySection
        t={t}
        hc={hc}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
      />
      <CpuSection
        t={t}
        hc={hc}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
      />
      <BlockIoSection
        t={t}
        hc={hc}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
      />
      <RuntimeOptionsSection
        t={t}
        hc={hc}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
      />
      <GpuSection
        t={t}
        editing={editing}
        editData={editData}
        gpuRequests={gpuRequests}
        onFieldChange={onFieldChange}
      />
    </div>
  );
}
