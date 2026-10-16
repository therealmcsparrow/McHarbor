// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';
import { EditInput } from './EditFieldControls';
import { InlineEditField } from '../InlineEditField';
import type { ResourceEditField } from '../../types/edit-form';

type Translator = (key: string) => string;

type ChangeHandler = <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;

type InlineSaveHandler = (field: ResourceEditField, value: number | string) => Promise<void>;

export function CpuSection({
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
        {t('resources.cpu')}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <EditInput
            label={t('resources.nanoCpus')}
            numeric
            value={editData?.nanoCpus ?? 0}
            onChange={(value) => onFieldChange('nanoCpus', parseInt(value, 10) || 0)}
          />
          <EditInput
            label={t('resources.cpuShares')}
            numeric
            value={editData?.cpuShares ?? 0}
            onChange={(value) => onFieldChange('cpuShares', parseInt(value, 10) || 0)}
          />
          <EditInput
            label={t('resources.cpuPeriod')}
            numeric
            value={editData?.cpuPeriod ?? 0}
            onChange={(value) => onFieldChange('cpuPeriod', parseInt(value, 10) || 0)}
            suffix="us"
          />
          <EditInput
            label={t('resources.cpuQuota')}
            numeric
            value={editData?.cpuQuota ?? 0}
            onChange={(value) => onFieldChange('cpuQuota', parseInt(value, 10) || 0)}
            suffix="us"
          />
          <EditInput
            label={t('resources.cpusetCpus')}
            value={editData?.cpusetCpus ?? ''}
            onChange={(value) => onFieldChange('cpusetCpus', value)}
            placeholder="0-3"
          />
        </div>
      ) : (
        <>
          <InlineEditField
            label={t('resources.nanoCpus')}
            numeric
            value={String(hc?.NanoCpus ?? 0)}
            displayValue={hc?.NanoCpus ? `${(hc.NanoCpus / 1e9).toFixed(2)} cores` : t('resources.unlimited')}
            disabled={!onInlineFieldSave}
            onSave={(value) => onInlineFieldSave?.('nanoCpus', Number(value)) ?? Promise.resolve()}
          />
          <InlineEditField
            label={t('resources.cpuShares')}
            numeric
            value={String(hc?.CpuShares ?? 0)}
            displayValue={hc?.CpuShares || '-'}
            disabled={!onInlineFieldSave}
            onSave={(value) => onInlineFieldSave?.('cpuShares', Number(value)) ?? Promise.resolve()}
          />
          <InlineEditField
            label={t('resources.cpuPeriod')}
            numeric
            value={String(hc?.CpuPeriod ?? 0)}
            displayValue={hc?.CpuPeriod ? `${hc.CpuPeriod} us` : '-'}
            disabled={!onInlineFieldSave}
            onSave={(value) => onInlineFieldSave?.('cpuPeriod', Number(value)) ?? Promise.resolve()}
          />
          <InlineEditField
            label={t('resources.cpuQuota')}
            numeric
            value={String(hc?.CpuQuota ?? 0)}
            displayValue={hc?.CpuQuota ? `${hc.CpuQuota} us` : '-'}
            disabled={!onInlineFieldSave}
            onSave={(value) => onInlineFieldSave?.('cpuQuota', Number(value)) ?? Promise.resolve()}
          />
          <InlineEditField
            label={t('resources.cpusetCpus')}
            value={hc?.CpusetCpus ?? ''}
            displayValue={hc?.CpusetCpus || '-'}
            disabled={!onInlineFieldSave}
            onSave={(value) => onInlineFieldSave?.('cpusetCpus', value) ?? Promise.resolve()}
          />
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
        {t('resources.blockIO')}
      </h3>
      {editing ? (
        <EditInput
          label={t('resources.blkioWeight')}
          numeric
          value={editData?.blkioWeight ?? 0}
          onChange={(value) => onFieldChange('blkioWeight', parseInt(value, 10) || 0)}
          placeholder="0-1000"
        />
      ) : (
        <InlineEditField
          label={t('resources.blkioWeight')}
          numeric
          value={String(hc?.BlkioWeight ?? 0)}
          displayValue={hc?.BlkioWeight || '-'}
          disabled={!onInlineFieldSave}
          onSave={(value) => onInlineFieldSave?.('blkioWeight', Number(value)) ?? Promise.resolve()}
        />
      )}
    </div>
  );
}
