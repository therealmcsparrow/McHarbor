// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { formatBytes } from '@resources/utils/format';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';
import { ScanSection } from './ScanSection';

type SecurityTabProps = {
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
};

const COMMON_CAPABILITIES = [
  'SYS_ADMIN', 'SYS_PTRACE', 'SYS_RAWIO', 'NET_ADMIN', 'NET_RAW',
  'IPC_LOCK', 'SYS_TIME', 'SYS_RESOURCE', 'MKNOD', 'AUDIT_WRITE',
  'SETFCAP', 'CHOWN', 'DAC_OVERRIDE', 'FOWNER', 'FSETID', 'KILL',
  'SETGID', 'SETUID', 'SETPCAP', 'NET_BIND_SERVICE', 'SYS_CHROOT',
];

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
      <Switch
        checked={checked}
        aria-label={label}
        onCheckedChange={onChange}
      />
    </div>
  );
}

function CapabilitySelector({
  label,
  selected,
  onChange,
}: {
  label: string;
  selected: string[];
  onChange: (caps: string[]) => void;
}) {
  return (
    <div>
      <label className="mb-2 block text-xs font-medium text-muted-foreground">{label}</label>
      <div className="flex flex-wrap gap-1.5">
        {COMMON_CAPABILITIES.map((cap) => {
          const isSelected = selected.includes(cap);
          return (
            <Button
              key={cap}
              type="button"
              variant={isSelected ? 'default' : 'outline'}
              size="sm"
              aria-pressed={isSelected}
              onClick={() => {
                if (isSelected) {
                  onChange(selected.filter((c) => c !== cap));
                } else {
                  onChange([...selected, cap]);
                }
              }}
              className="h-6 rounded-md px-2 text-[10px]"
            >
              {cap}
            </Button>
          );
        })}
      </div>
    </div>
  );
}

export function SecurityTab({ container, editing, editData, onFieldChange }: SecurityTabProps) {
  const { t } = useTranslation('containers');
  const hc = container.HostConfig;

  const imageRef = container.Config?.Image ?? '';

  return (
    <div className="grid grid-cols-1 gap-6">
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('resources.security')}</h3>
        {editing ? (
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <ToggleField
                label={t('resources.privileged')}
                checked={editData?.privileged ?? false}
                onChange={(v) => onFieldChange('privileged', v)}
              />
              <ToggleField
                label={t('resources.readonlyRootfs')}
                checked={editData?.readonlyRootfs ?? false}
                onChange={(v) => onFieldChange('readonlyRootfs', v)}
              />
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <EditInput
                label={t('resources.pidMode')}
                value={editData?.pidMode ?? ''}
                onChange={(v) => onFieldChange('pidMode', v)}
                placeholder="host"
              />
              <EditInput
                label={t('resources.shmSize')}
                type="number"
                value={editData?.shmSize ?? 0}
                onChange={(v) => onFieldChange('shmSize', parseInt(v) || 0)}
                suffix="bytes"
              />
            </div>
            <CapabilitySelector
              label={t('resources.capAdd')}
              selected={editData?.capAdd ?? []}
              onChange={(caps) => onFieldChange('capAdd', caps)}
            />
            <CapabilitySelector
              label={t('resources.capDrop')}
              selected={editData?.capDrop ?? []}
              onChange={(caps) => onFieldChange('capDrop', caps)}
            />
            <EditInput
              label={t('resources.securityOpt')}
              value={editData?.securityOpt?.join(', ') ?? ''}
              onChange={(v) => onFieldChange('securityOpt', v ? v.split(',').map((s) => s.trim()) : [])}
              placeholder="no-new-privileges, apparmor=unconfined"
            />
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-x-8 sm:grid-cols-2">
            <InfoRow label={t('resources.privileged')}>
              {hc?.Privileged ? (
                <Badge variant="destructive">{t('common:labels.yes')}</Badge>
              ) : (
                t('common:labels.no')
              )}
            </InfoRow>
            <InfoRow label={t('resources.readonlyRootfs')}>
              {hc?.ReadonlyRootfs ? t('common:labels.yes') : t('common:labels.no')}
            </InfoRow>
            <InfoRow label={t('resources.capAdd')}>
              {hc?.CapAdd?.length ? (
                <div className="flex flex-wrap gap-1">
                  {hc.CapAdd.map((c) => (
                    <Badge key={c} variant="success" className="text-[10px]">{c}</Badge>
                  ))}
                </div>
              ) : '-'}
            </InfoRow>
            <InfoRow label={t('resources.capDrop')}>
              {hc?.CapDrop?.length ? (
                <div className="flex flex-wrap gap-1">
                  {hc.CapDrop.map((c) => (
                    <Badge key={c} variant="destructive" className="text-[10px]">{c}</Badge>
                  ))}
                </div>
              ) : '-'}
            </InfoRow>
            <InfoRow label={t('resources.pidMode')}>{hc?.PidMode || '-'}</InfoRow>
            <InfoRow label={t('resources.shmSize')}>
              {hc?.ShmSize ? formatBytes(hc.ShmSize) : '-'}
            </InfoRow>
            {hc?.SecurityOpt && hc.SecurityOpt.length > 0 && (
              <InfoRow label={t('resources.securityOpt')}>
                <div className="flex flex-wrap gap-1">
                  {hc.SecurityOpt.map((s) => (
                    <Badge key={s} variant="secondary" className="text-[10px] font-mono">{s}</Badge>
                  ))}
                </div>
              </InfoRow>
            )}
            {hc?.UsernsMode && (
              <InfoRow label={t('resources.usernsMode')}>{hc.UsernsMode}</InfoRow>
            )}
          </div>
        )}
      </div>

      {/* Vulnerability scan section */}
      {imageRef && <ScanSection imageRef={imageRef} />}
    </div>
  );
}
