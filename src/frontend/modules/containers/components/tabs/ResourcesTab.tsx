// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Badge } from '@resources/components/ui/Badge';
import { formatBytes } from '@resources/utils/format';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';

type ResourcesTabProps = {
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
};

const RESTART_POLICIES = ['no', 'always', 'unless-stopped', 'on-failure'];

function splitCsv(value: string): string[] {
  return value.split(',').map((item) => item.trim()).filter(Boolean);
}

function EditInput({
  label,
  value,
  onChange,
  type = 'text',
  suffix,
  placeholder,
}: {
  label: string;
  value: string | number;
  onChange: (val: string) => void;
  type?: string;
  suffix?: string;
  placeholder?: string;
}) {
  return (
    <div>
      <label className="text-xs font-medium text-muted-foreground">{label}</label>
      <div className="mt-1 flex items-center gap-2">
        <input
          type={type}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          className="w-full rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
        />
        {suffix && <span className="shrink-0 text-xs text-muted-foreground">{suffix}</span>}
      </div>
    </div>
  );
}

function ToggleField({
  label,
  checked,
  onChange,
  description,
}: {
  label: string;
  checked: boolean;
  onChange: (val: boolean) => void;
  description?: string;
}) {
  return (
    <div className="flex items-center justify-between">
      <div>
        <span className="text-xs font-medium text-muted-foreground">{label}</span>
        {description && <p className="text-[10px] text-muted-foreground/70">{description}</p>}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        onClick={() => onChange(!checked)}
        className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors ${
          checked ? 'bg-primary' : 'bg-muted-foreground/30'
        }`}
      >
        <span className={`pointer-events-none inline-block size-4 rounded-full bg-white shadow-sm transition-transform ${
          checked ? 'translate-x-4' : 'translate-x-0'
        }`} />
      </button>
    </div>
  );
}

export function ResourcesTab({ container, editing, editData, onFieldChange }: ResourcesTabProps) {
  const { t } = useTranslation('containers');
  const hc = container.HostConfig;
  const gpuRequests = editing
    ? (editData?.gpuEnabled
        ? [{
            Driver: editData.gpuDriver,
            Count: editData.gpuDeviceIds.length > 0 ? 0 : editData.gpuCount,
            DeviceIDs: editData.gpuDeviceIds,
            Capabilities: [editData.gpuCapabilities.length > 0 ? editData.gpuCapabilities : ['gpu']],
          }]
        : [])
    : (hc?.DeviceRequests ?? []);

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      {/* Restart Policy */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('resources.restartPolicy')}</h3>
        {editing ? (
          <div className="space-y-3">
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('resources.policy')}</label>
              <select
                value={editData?.restartPolicyName ?? 'no'}
                onChange={(e) => onFieldChange('restartPolicyName', e.target.value)}
                className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
              >
                {RESTART_POLICIES.map((p) => (
                  <option key={p} value={p}>{p}</option>
                ))}
              </select>
            </div>
            {editData?.restartPolicyName === 'on-failure' && (
              <EditInput
                label={t('resources.maxRetries')}
                type="number"
                value={editData?.restartPolicyMaxRetry ?? 0}
                onChange={(v) => onFieldChange('restartPolicyMaxRetry', parseInt(v) || 0)}
              />
            )}
          </div>
        ) : (
          <>
            <InfoRow label={t('resources.policy')}>{hc?.RestartPolicy?.Name ?? 'no'}</InfoRow>
            {hc?.RestartPolicy?.Name === 'on-failure' && (
              <InfoRow label={t('resources.maxRetries')}>{hc?.RestartPolicy?.MaximumRetryCount ?? 0}</InfoRow>
            )}
          </>
        )}
      </div>

      {/* Memory */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('resources.memory')}</h3>
        {editing ? (
          <div className="space-y-3">
            <EditInput
              label={t('resources.memoryLimit')}
              type="number"
              value={editData?.memory ?? 0}
              onChange={(v) => onFieldChange('memory', parseInt(v) || 0)}
              suffix="bytes"
            />
            <EditInput
              label={t('resources.memorySwap')}
              type="number"
              value={editData?.memorySwap ?? 0}
              onChange={(v) => onFieldChange('memorySwap', parseInt(v) || 0)}
              suffix="bytes"
            />
            <EditInput
              label={t('resources.memoryReservation')}
              type="number"
              value={editData?.memoryReservation ?? 0}
              onChange={(v) => onFieldChange('memoryReservation', parseInt(v) || 0)}
              suffix="bytes"
            />
          </div>
        ) : (
          <>
            <InfoRow label={t('resources.memoryLimit')}>
              {hc?.Memory ? formatBytes(hc.Memory) : t('resources.unlimited')}
            </InfoRow>
            <InfoRow label={t('resources.memorySwap')}>
              {hc?.MemorySwap && hc.MemorySwap > 0 ? formatBytes(hc.MemorySwap) : t('resources.unlimited')}
            </InfoRow>
            <InfoRow label={t('resources.memoryReservation')}>
              {hc?.MemoryReservation ? formatBytes(hc.MemoryReservation) : '-'}
            </InfoRow>
          </>
        )}
      </div>

      {/* CPU */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('resources.cpu')}</h3>
        {editing ? (
          <div className="space-y-3">
            <EditInput
              label={t('resources.nanoCpus')}
              type="number"
              value={editData?.nanoCpus ?? 0}
              onChange={(v) => onFieldChange('nanoCpus', parseInt(v) || 0)}
            />
            <EditInput
              label={t('resources.cpuShares')}
              type="number"
              value={editData?.cpuShares ?? 0}
              onChange={(v) => onFieldChange('cpuShares', parseInt(v) || 0)}
            />
            <EditInput
              label={t('resources.cpuPeriod')}
              type="number"
              value={editData?.cpuPeriod ?? 0}
              onChange={(v) => onFieldChange('cpuPeriod', parseInt(v) || 0)}
              suffix="µs"
            />
            <EditInput
              label={t('resources.cpuQuota')}
              type="number"
              value={editData?.cpuQuota ?? 0}
              onChange={(v) => onFieldChange('cpuQuota', parseInt(v) || 0)}
              suffix="µs"
            />
            <EditInput
              label={t('resources.cpusetCpus')}
              value={editData?.cpusetCpus ?? ''}
              onChange={(v) => onFieldChange('cpusetCpus', v)}
              placeholder="0-3"
            />
          </div>
        ) : (
          <>
            <InfoRow label={t('resources.nanoCpus')}>
              {hc?.NanoCpus ? `${(hc.NanoCpus / 1e9).toFixed(2)} cores` : t('resources.unlimited')}
            </InfoRow>
            <InfoRow label={t('resources.cpuShares')}>{hc?.CpuShares || '-'}</InfoRow>
            <InfoRow label={t('resources.cpuPeriod')}>{hc?.CpuPeriod ? `${hc.CpuPeriod} µs` : '-'}</InfoRow>
            <InfoRow label={t('resources.cpuQuota')}>{hc?.CpuQuota ? `${hc.CpuQuota} µs` : '-'}</InfoRow>
            <InfoRow label={t('resources.cpusetCpus')}>{hc?.CpusetCpus || '-'}</InfoRow>
          </>
        )}
      </div>

      {/* Block I/O */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('resources.blockIO')}</h3>
        {editing ? (
          <EditInput
            label={t('resources.blkioWeight')}
            type="number"
            value={editData?.blkioWeight ?? 0}
            onChange={(v) => onFieldChange('blkioWeight', parseInt(v) || 0)}
            placeholder="0-1000"
          />
        ) : (
          <InfoRow label={t('resources.blkioWeight')}>{hc?.BlkioWeight || '-'}</InfoRow>
        )}
      </div>

      {/* Runtime Options */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('resources.runtimeOptions')}</h3>
        {editing ? (
          <div className="space-y-3">
            <ToggleField
              label={t('resources.autoRemove')}
              checked={editData?.autoRemove ?? false}
              onChange={(v) => onFieldChange('autoRemove', v)}
            />
            <ToggleField
              label={t('resources.init')}
              checked={editData?.init ?? false}
              onChange={(v) => onFieldChange('init', v)}
            />
            <ToggleField
              label={t('resources.oomKillDisable')}
              checked={editData?.oomKillDisable ?? false}
              onChange={(v) => onFieldChange('oomKillDisable', v)}
            />
            <EditInput
              label={t('resources.pidsLimit')}
              type="number"
              value={editData?.pidsLimit ?? 0}
              onChange={(v) => onFieldChange('pidsLimit', parseInt(v) || 0)}
              placeholder="0 = unlimited"
            />
          </div>
        ) : (
          <>
            <InfoRow label={t('resources.autoRemove')}>
              {hc?.AutoRemove ? <Badge variant="warning">{t('common:labels.yes')}</Badge> : t('common:labels.no')}
            </InfoRow>
            <InfoRow label={t('resources.init')}>
              {hc?.Init ? t('common:labels.yes') : t('common:labels.no')}
            </InfoRow>
            <InfoRow label={t('resources.oomKillDisable')}>
              {hc?.OomKillDisable ? <Badge variant="destructive">{t('common:labels.yes')}</Badge> : t('common:labels.no')}
            </InfoRow>
            <InfoRow label={t('resources.pidsLimit')}>
              {hc?.PidsLimit && hc.PidsLimit > 0 ? hc.PidsLimit : t('resources.unlimited')}
            </InfoRow>
          </>
        )}
      </div>

      {/* GPU / Device Requests */}
      {(editing || gpuRequests.length > 0) && (
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('overview.gpu')}</h3>
          {editing ? (
            <div className="space-y-3">
              <ToggleField
                label={t('resources.enableGpu')}
                checked={editData?.gpuEnabled ?? false}
                onChange={(value) => onFieldChange('gpuEnabled', value)}
              />
              {editData?.gpuEnabled && (
                <>
                  <EditInput
                    label={t('overview.gpuDriver')}
                    value={editData.gpuDriver}
                    onChange={(value) => onFieldChange('gpuDriver', value)}
                    placeholder="nvidia"
                  />
                  <EditInput
                    label={t('overview.gpuCount')}
                    type="number"
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
                    placeholder="gpu"
                  />
                  <p className="text-[10px] text-muted-foreground/70">{t('resources.gpuCapabilitiesHelp')}</p>
                </>
              )}
            </div>
          ) : (
            gpuRequests.map((dr, idx) => (
              <div key={`dr-${dr.Driver}-${idx}`} className="space-y-1">
                <InfoRow label={t('overview.gpuDriver')}>{dr.Driver || '-'}</InfoRow>
                <InfoRow label={t('overview.gpuCount')}>{dr.Count === -1 ? t('common:labels.all') : String(dr.Count)}</InfoRow>
                {dr.DeviceIDs && dr.DeviceIDs.length > 0 && (
                  <InfoRow label={t('overview.gpuDeviceIDs')}>{dr.DeviceIDs.join(', ')}</InfoRow>
                )}
                {dr.Capabilities && dr.Capabilities.length > 0 && (
                  <InfoRow label={t('overview.gpuCapabilities')}>
                    {dr.Capabilities.flat().join(', ')}
                  </InfoRow>
                )}
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}
