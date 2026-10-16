// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData, ResourceEditField } from '../../types/edit-form';
import { MemorySection, RestartPolicySection } from './ResourcesSections';
import { BlockIoSection, CpuSection } from './ResourcesCpuSections';
import { GpuSection, RuntimeOptionsSection } from './ResourcesRuntimeSections';
import { DevicesSection } from './DevicesSection';

type ResourcesTabProps = {
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
  onInlineFieldSave?: (field: ResourceEditField, value: number | string) => Promise<void>;
};

export function ResourcesTab({ container, editing, editData, onFieldChange, onInlineFieldSave }: ResourcesTabProps) {
  const { t } = useTranslation('containers');
  const hc = container.HostConfig;
  const gpuRequests = editing
    ? editData?.gpuEnabled
      ? [
          {
            Driver: editData.gpuDriver,
            Count: editData.gpuDeviceIds.length > 0 ? 0 : editData.gpuCount,
            DeviceIDs: editData.gpuDeviceIds,
            Capabilities: [editData.gpuCapabilities.length > 0 ? editData.gpuCapabilities : ['gpu']],
            Options: null,
          },
        ]
      : []
    : (hc?.DeviceRequests ?? []);

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <RestartPolicySection t={t} hc={hc} editing={editing} editData={editData} onFieldChange={onFieldChange} />
      <MemorySection
        t={t}
        hc={hc}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
        onInlineFieldSave={onInlineFieldSave}
      />
      <CpuSection
        t={t}
        hc={hc}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
        onInlineFieldSave={onInlineFieldSave}
      />
      <BlockIoSection
        t={t}
        hc={hc}
        editing={editing}
        editData={editData}
        onFieldChange={onFieldChange}
        onInlineFieldSave={onInlineFieldSave}
      />
      <RuntimeOptionsSection t={t} hc={hc} editing={editing} editData={editData} onFieldChange={onFieldChange} />
      <GpuSection t={t} editing={editing} editData={editData} gpuRequests={gpuRequests} onFieldChange={onFieldChange} />
      <DevicesSection t={t} hc={hc} editing={editing} editData={editData} onFieldChange={onFieldChange} />
    </div>
  );
}
