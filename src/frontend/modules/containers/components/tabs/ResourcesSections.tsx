// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { InfoRow } from '@resources/components/ui/InfoRow';
import { Select } from '@resources/components/ui/Select';
import { formatBytes } from '@resources/utils/format';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';
import { EditInput } from './EditFieldControls';
import { InlineEditField } from '../InlineEditField';
import type { ResourceEditField } from '../../types/edit-form';

type Translator = (key: string) => string;

type ChangeHandler = <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;

type InlineSaveHandler = (field: ResourceEditField, value: number | string) => Promise<void>;

const NO_RESTART_POLICY = 'no';

export const RESTART_POLICIES = ['no', 'always', 'unless-stopped', 'on-failure'];

export function splitCsv(value: string): string[] {
  return value
    .split(',')
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
  hc: ContainerInspect['HostConfig'];
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('resources.restartPolicy')}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <div>
            <label className="text-xs font-medium text-muted-foreground">{t('resources.policy')}</label>
            <Select
              value={editData?.restartPolicyName ?? 'no'}
              onChange={(next) => onFieldChange('restartPolicyName', next)}
              options={RESTART_POLICIES.map((policy) => ({
                value: policy,
                label: policy,
              }))}
              searchable={false}
              ariaLabel={t('resources.policy')}
              variant="outline"
              className="mt-1 text-xs"
            />
          </div>
          {editData?.restartPolicyName === 'on-failure' && (
            <EditInput
              label={t('resources.maxRetries')}
              numeric
              value={editData?.restartPolicyMaxRetry ?? 0}
              onChange={(value) => onFieldChange('restartPolicyMaxRetry', parseInt(value, 10) || 0)}
            />
          )}
        </div>
      ) : (
        <>
          <InfoRow label={t('resources.policy')}>{hc?.RestartPolicy?.Name ?? NO_RESTART_POLICY}</InfoRow>
          {hc?.RestartPolicy?.Name === 'on-failure' && (
            <InfoRow label={t('resources.maxRetries')}>{hc?.RestartPolicy?.MaximumRetryCount ?? 0}</InfoRow>
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
  onInlineFieldSave,
}: {
  t: Translator;
  hc: ContainerInspect['HostConfig'];
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
  onInlineFieldSave?: InlineSaveHandler;
}) {
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('resources.memory')}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <EditInput
            label={t('resources.memoryLimit')}
            numeric
            value={editData?.memory ?? 0}
            onChange={(value) => onFieldChange('memory', parseInt(value, 10) || 0)}
            suffix="bytes"
          />
          <EditInput
            label={t('resources.memorySwap')}
            numeric
            value={editData?.memorySwap ?? 0}
            onChange={(value) => onFieldChange('memorySwap', parseInt(value, 10) || 0)}
            suffix="bytes"
          />
          <EditInput
            label={t('resources.memoryReservation')}
            numeric
            value={editData?.memoryReservation ?? 0}
            onChange={(value) => onFieldChange('memoryReservation', parseInt(value, 10) || 0)}
            suffix="bytes"
          />
        </div>
      ) : (
        <>
          <InlineEditField
            label={t('resources.memoryLimit')}
            numeric
            value={String(hc?.Memory ?? 0)}
            displayValue={hc?.Memory ? formatBytes(hc.Memory) : t('resources.unlimited')}
            disabled={!onInlineFieldSave}
            onSave={(value) => onInlineFieldSave?.('memory', Number(value)) ?? Promise.resolve()}
          />
          <InlineEditField
            label={t('resources.memorySwap')}
            numeric
            value={String(hc?.MemorySwap ?? 0)}
            displayValue={hc?.MemorySwap && hc.MemorySwap > 0 ? formatBytes(hc.MemorySwap) : t('resources.unlimited')}
            disabled={!onInlineFieldSave}
            onSave={(value) => onInlineFieldSave?.('memorySwap', Number(value)) ?? Promise.resolve()}
          />
          <InlineEditField
            label={t('resources.memoryReservation')}
            numeric
            value={String(hc?.MemoryReservation ?? 0)}
            displayValue={hc?.MemoryReservation ? formatBytes(hc.MemoryReservation) : '-'}
            disabled={!onInlineFieldSave}
            onSave={(value) => onInlineFieldSave?.('memoryReservation', Number(value)) ?? Promise.resolve()}
          />
        </>
      )}
    </div>
  );
}
