// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Badge } from '@resources/components/ui/Badge';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';
import { EditInput, ToggleField } from './EditFieldControls';
import { GpuDriverField } from '../GpuDriverField';
import { splitCsv } from './ResourcesSections';

type Translator = (key: string) => string;

type ChangeHandler = <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;

type DeviceRequest = NonNullable<NonNullable<ContainerInspect['HostConfig']>['DeviceRequests']>[number];

export function RuntimeOptionsSection({
  t,
  hc,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  hc: ContainerInspect['HostConfig'];
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('resources.runtimeOptions')}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <ToggleField
            label={t('resources.autoRemove')}
            checked={editData?.autoRemove ?? false}
            onChange={(value) => onFieldChange('autoRemove', value)}
          />
          <ToggleField
            label={t('resources.init')}
            checked={editData?.init ?? false}
            onChange={(value) => onFieldChange('init', value)}
          />
          <ToggleField
            label={t('resources.oomKillDisable')}
            checked={editData?.oomKillDisable ?? false}
            onChange={(value) => onFieldChange('oomKillDisable', value)}
          />
          <EditInput
            label={t('resources.pidsLimit')}
            numeric
            value={editData?.pidsLimit ?? 0}
            onChange={(value) => onFieldChange('pidsLimit', parseInt(value, 10) || 0)}
            placeholder={t('placeholders.pidsLimit')}
          />
        </div>
      ) : (
        <>
          <InfoRow label={t('resources.autoRemove')}>
            {hc?.AutoRemove ? <Badge variant="warning">{t('common:labels.yes')}</Badge> : t('common:labels.no')}
          </InfoRow>
          <InfoRow label={t('resources.init')}>{hc?.Init ? t('common:labels.yes') : t('common:labels.no')}</InfoRow>
          <InfoRow label={t('resources.oomKillDisable')}>
            {hc?.OomKillDisable ? <Badge variant="destructive">{t('common:labels.yes')}</Badge> : t('common:labels.no')}
          </InfoRow>
          <InfoRow label={t('resources.pidsLimit')}>
            {hc?.PidsLimit && hc.PidsLimit > 0 ? hc.PidsLimit : t('resources.unlimited')}
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
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">{t('overview.gpu')}</h3>
      {editing ? (
        <div className="space-y-3">
          <ToggleField
            label={t('resources.enableGpu')}
            checked={editData?.gpuEnabled ?? false}
            onChange={(value) => onFieldChange('gpuEnabled', value)}
          />
          {editData?.gpuEnabled && (
            <>
              <GpuDriverField
                id="edit-gpu-driver"
                value={editData.gpuDriver}
                onChange={(value) => onFieldChange('gpuDriver', value)}
              />
              <EditInput
                label={t('overview.gpuCount')}
                numeric
                value={editData.gpuCount}
                onChange={(value) => onFieldChange('gpuCount', parseInt(value, 10) || 0)}
              />
              <p className="text-[10px] text-muted-foreground/70">{t('resources.gpuCountHelp')}</p>
              <EditInput
                label={t('overview.gpuDeviceIDs')}
                value={editData.gpuDeviceIds.join(', ')}
                onChange={(value) => onFieldChange('gpuDeviceIds', splitCsv(value))}
                placeholder="0,1"
              />
              <p className="text-[10px] text-muted-foreground/70">{t('resources.gpuDeviceIdsHelp')}</p>
              <EditInput
                label={t('overview.gpuCapabilities')}
                value={editData.gpuCapabilities.join(', ')}
                onChange={(value) => onFieldChange('gpuCapabilities', splitCsv(value))}
                placeholder={t('placeholders.gpuCapability')}
              />
              <p className="text-[10px] text-muted-foreground/70">{t('resources.gpuCapabilitiesHelp')}</p>
            </>
          )}
        </div>
      ) : (
        gpuRequests.map((request, index) => (
          <div key={`dr-${request.Driver}-${index}`} className="space-y-1">
            <InfoRow label={t('overview.gpuDriver')}>{request.Driver || '-'}</InfoRow>
            <InfoRow label={t('overview.gpuCount')}>
              {request.Count === -1 ? t('common:labels.all') : String(request.Count)}
            </InfoRow>
            {request.DeviceIDs && request.DeviceIDs.length > 0 && (
              <InfoRow label={t('overview.gpuDeviceIDs')}>{request.DeviceIDs.join(', ')}</InfoRow>
            )}
            {request.Capabilities && request.Capabilities.length > 0 && (
              <InfoRow label={t('overview.gpuCapabilities')}>{request.Capabilities.flat().join(', ')}</InfoRow>
            )}
          </div>
        ))
      )}
    </div>
  );
}
