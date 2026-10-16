// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// GPU access for the create dialog.
//
// Extracted from `CreateContainerDialog.tsx` so that file can stay under
// the 200-line-per-concern guidance, and mirrors the Resources tab's GPU
// editor field for field — the two write the same `DeviceRequests` shape,
// so a container created here reads back unchanged when edited.

import { useTranslation } from 'react-i18next';
import { IconCpu2 } from '@tabler/icons-react';
import { Section } from '@resources/components/ui/Form';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { Text as BodyText } from '@resources/components/ui/Typography';
import type { GpuRequest } from '../hooks/useCreateContainer';
import { GpuDriverField } from './GpuDriverField';

const DEVICE_IDS_EXAMPLE = '0, 1';

type CreateContainerGpuSectionProps = {
  value: GpuRequest;
  onChange: (next: GpuRequest) => void;
};

/** Split "0, 1" into ["0","1"], dropping the blanks a trailing comma leaves. */
function splitCsv(raw: string): string[] {
  return raw
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean);
}

export function CreateContainerGpuSection({ value, onChange }: CreateContainerGpuSectionProps) {
  const { t } = useTranslation('containers');
  const set = <K extends keyof GpuRequest>(key: K, next: GpuRequest[K]) => onChange({ ...value, [key]: next });

  return (
    <Section icon={<IconCpu2 className="size-3.5" />} title={t('overview.gpu')} help={t('create.form.gpuHelp')}>
      <div className="space-y-3">
        <label className="flex items-center gap-3 text-sm text-foreground">
          <Switch checked={value.enabled} onCheckedChange={(checked) => set('enabled', checked)} />
          {t('resources.enableGpu')}
        </label>

        {value.enabled && (
          <div className="space-y-3">
            <GpuDriverField id="create-gpu-driver" value={value.driver} onChange={(next) => set('driver', next)} />

            <div>
              <Label htmlFor="create-gpu-count" className="mb-1 text-xs text-muted-foreground">
                {t('overview.gpuCount')}
              </Label>
              <NumberInput
                id="create-gpu-count"
                min={0}
                value={value.count}
                onChange={(count) => set('count', Math.trunc(count))}
                // Device IDs win over a count in Docker's API, so the
                // field is disabled rather than silently ignored once
                // specific devices are named.
                disabled={value.deviceIds.length > 0}
              />
              <BodyText variant="muted" size="sm" className="mt-1 text-xs">
                {value.deviceIds.length > 0 ? t('resources.gpuCountIgnored') : t('resources.gpuCountHelp')}
              </BodyText>
            </div>

            <div>
              <Label htmlFor="create-gpu-devices" className="mb-1 text-xs text-muted-foreground">
                {t('overview.gpuDeviceIDs')}
              </Label>
              <Input
                id="create-gpu-devices"
                variant="outline"
                value={value.deviceIds.join(', ')}
                onChange={(changeEvent) => set('deviceIds', splitCsv(changeEvent.target.value))}
                placeholder={DEVICE_IDS_EXAMPLE}
              />
              <BodyText variant="muted" size="sm" className="mt-1 text-xs">
                {t('resources.gpuDeviceIdsHelp')}
              </BodyText>
            </div>

            <div>
              <Label htmlFor="create-gpu-capabilities" className="mb-1 text-xs text-muted-foreground">
                {t('overview.gpuCapabilities')}
              </Label>
              <Input
                id="create-gpu-capabilities"
                variant="outline"
                value={value.capabilities.join(', ')}
                onChange={(evt) => set('capabilities', splitCsv(evt.target.value))}
                placeholder={t('placeholders.gpuCapability')}
              />
              <BodyText variant="muted" size="sm" className="mt-1 text-xs">
                {t('resources.gpuCapabilitiesHelp')}
              </BodyText>
            </div>
          </div>
        )}
      </div>
    </Section>
  );
}
