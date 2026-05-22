// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { KeyValueEditor } from '../KeyValueEditor';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData, HealthcheckConfig } from '../../types/edit-form';

type EnvironmentTabProps = {
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
};

function EditInput({
  label,
  value,
  onChange,
  type = 'text',
  placeholder,
}: {
  label: string;
  value: string | number;
  onChange: (val: string) => void;
  type?: string;
  placeholder?: string;
}) {
  return (
    <div>
      <label className="text-xs font-medium text-muted-foreground">{label}</label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
      />
    </div>
  );
}

function ToggleField({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (val: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
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

export function EnvironmentTab({ container, editing, editData, onFieldChange }: EnvironmentTabProps) {
  const { t } = useTranslation('containers');

  const envEntries = useMemo(() => {
    const envList = editing ? (editData?.env ?? []) : (container.Config?.Env ?? []);
    return envList.map((e) => {
      const idx = e.indexOf('=');
      return { key: idx > -1 ? e.slice(0, idx) : e, value: idx > -1 ? e.slice(idx + 1) : '' };
    });
  }, [editing, editData?.env, container.Config?.Env]);


  const logOptionEntries = useMemo(() => {
    const opts = editing ? (editData?.logOptions ?? {}) : (container.HostConfig?.LogConfig?.Config ?? {});
    return Object.entries(opts).map(([key, value]) => ({ key, value }));
  }, [editing, editData?.logOptions, container.HostConfig?.LogConfig?.Config]);

  const handleEnvChange = useCallback(
    (entries: Array<{ key: string; value: string }>) => {
      onFieldChange('env', entries.map((e) => `${e.key}=${e.value}`));
    },
    [onFieldChange],
  );

  const handleLogOptionChange = useCallback(
    (entries: Array<{ key: string; value: string }>) => {
      onFieldChange('logOptions', Object.fromEntries(entries.map((e) => [e.key, e.value])));
    },
    [onFieldChange],
  );

  const handleHealthcheckChange = useCallback(
    <K extends keyof HealthcheckConfig>(field: K, value: HealthcheckConfig[K]) => {
      onFieldChange('healthcheck', { ...(editData?.healthcheck ?? { enabled: false, command: '', interval: 30, timeout: 30, retries: 3, startPeriod: 0 }), [field]: value });
    },
    [editData?.healthcheck, onFieldChange],
  );

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      {/* Image (edit only) */}
      {editing && (
        <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('environment.image')}</h3>
          <EditInput
            label={t('overview.image')}
            value={editData?.image ?? ''}
            onChange={(v) => onFieldChange('image', v)}
            placeholder="nginx:latest"
          />
        </div>
      )}

      {/* Environment Variables */}
      <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('environment.envVars')}</h3>
        {editing ? (
          <KeyValueEditor
            entries={envEntries}
            onChange={handleEnvChange}
            keyLabel={t('edit.key')}
            valueLabel={t('edit.value')}
            addLabel={t('edit.addEnvVar')}
          />
        ) : envEntries.length > 0 ? (
          <div className="max-h-64 overflow-y-auto">
            {envEntries.map((entry) => (
              <div key={`${entry.key}:${entry.value}`} className="flex gap-2 border-b border-border py-1.5 last:border-0">
                <span className="shrink-0 font-mono text-xs font-medium text-foreground">{entry.key}</span>
                <span className="truncate font-mono text-xs text-muted-foreground">{entry.value}</span>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('overview.noEnvVars')}</p>
        )}
      </div>

      {/* Config Fields */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('environment.config')}</h3>
        {editing ? (
          <div className="space-y-3">
            <EditInput
              label={t('environment.user')}
              value={editData?.user ?? ''}
              onChange={(v) => onFieldChange('user', v)}
              placeholder="user:group"
            />
            <EditInput
              label={t('environment.hostname')}
              value={editData?.hostname ?? ''}
              onChange={(v) => onFieldChange('hostname', v)}
            />
            <EditInput
              label={t('environment.domainname')}
              value={editData?.domainname ?? ''}
              onChange={(v) => onFieldChange('domainname', v)}
            />
            <EditInput
              label={t('environment.workingDir')}
              value={editData?.workingDir ?? ''}
              onChange={(v) => onFieldChange('workingDir', v)}
            />
            <EditInput
              label={t('environment.command')}
              value={editData?.cmd?.join(' ') ?? ''}
              onChange={(v) => onFieldChange('cmd', v ? v.split(' ') : [])}
            />
            <EditInput
              label={t('environment.entrypoint')}
              value={editData?.entrypoint?.join(' ') ?? ''}
              onChange={(v) => onFieldChange('entrypoint', v ? v.split(' ') : [])}
            />
            <EditInput
              label={t('environment.stopSignal')}
              value={editData?.stopSignal ?? ''}
              onChange={(v) => onFieldChange('stopSignal', v)}
              placeholder="SIGTERM"
            />
          </div>
        ) : (
          <>
            <InfoRow label={t('environment.user')}>{container.Config?.User || '-'}</InfoRow>
            <InfoRow label={t('environment.hostname')}>{container.Config?.Hostname ?? '-'}</InfoRow>
            <InfoRow label={t('environment.domainname')}>{container.Config?.Domainname || '-'}</InfoRow>
            <InfoRow label={t('environment.workingDir')}>{container.Config?.WorkingDir || '/'}</InfoRow>
            <InfoRow label={t('environment.command')}>
              <span className="font-mono text-xs">{container.Config?.Cmd?.join(' ') ?? '-'}</span>
            </InfoRow>
            <InfoRow label={t('environment.entrypoint')}>
              <span className="font-mono text-xs">{container.Config?.Entrypoint?.join(' ') ?? '-'}</span>
            </InfoRow>
            <InfoRow label={t('environment.stopSignal')}>{container.Config?.StopSignal || 'SIGTERM'}</InfoRow>
          </>
        )}
      </div>

      {/* Console & Logging */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('environment.console')}</h3>
        {editing ? (
          <div className="space-y-3">
            <ToggleField
              label={t('environment.tty')}
              checked={editData?.tty ?? false}
              onChange={(v) => onFieldChange('tty', v)}
            />
            <ToggleField
              label={t('environment.openStdin')}
              checked={editData?.openStdin ?? false}
              onChange={(v) => onFieldChange('openStdin', v)}
            />
            <EditInput
              label={t('environment.logDriver')}
              value={editData?.logDriver ?? ''}
              onChange={(v) => onFieldChange('logDriver', v)}
              placeholder="json-file"
            />
          </div>
        ) : (
          <>
            <InfoRow label={t('environment.tty')}>{container.Config?.Tty ? t('common:labels.yes') : t('common:labels.no')}</InfoRow>
            <InfoRow label={t('environment.openStdin')}>{container.Config?.OpenStdin ? t('common:labels.yes') : t('common:labels.no')}</InfoRow>
            <InfoRow label={t('environment.logDriver')}>{container.HostConfig?.LogConfig?.Type ?? 'json-file'}</InfoRow>
          </>
        )}
      </div>

      {/* Log Options */}
      {(editing || logOptionEntries.length > 0) && (
        <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('environment.logOptions')}</h3>
          {editing ? (
            <KeyValueEditor
              entries={logOptionEntries}
              onChange={handleLogOptionChange}
              keyLabel={t('edit.key')}
              valueLabel={t('edit.value')}
              addLabel={t('edit.addLogOption')}
            />
          ) : (
            <div className="max-h-48 overflow-y-auto">
              {logOptionEntries.map(({ key, value }) => (
                <div key={key} className="flex gap-2 border-b border-border py-1.5 last:border-0">
                  <span className="shrink-0 font-mono text-xs font-medium text-foreground">{key}</span>
                  <span className="truncate font-mono text-xs text-muted-foreground">{value}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Healthcheck */}
      {editing && (
        <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('environment.healthcheck')}</h3>
          <div className="space-y-3">
            <ToggleField
              label={t('environment.healthEnabled')}
              checked={editData?.healthcheck?.enabled ?? false}
              onChange={(v) => handleHealthcheckChange('enabled', v)}
            />
            {editData?.healthcheck?.enabled && (
              <>
                <EditInput
                  label={t('environment.healthCommand')}
                  value={editData.healthcheck.command}
                  onChange={(v) => handleHealthcheckChange('command', v)}
                  placeholder="curl -f http://localhost/ || exit 1"
                />
                <div className="grid grid-cols-2 gap-3">
                  <EditInput
                    label={t('environment.healthInterval')}
                    type="number"
                    value={editData.healthcheck.interval}
                    onChange={(v) => handleHealthcheckChange('interval', parseInt(v) || 30)}
                  />
                  <EditInput
                    label={t('environment.healthTimeout')}
                    type="number"
                    value={editData.healthcheck.timeout}
                    onChange={(v) => handleHealthcheckChange('timeout', parseInt(v) || 30)}
                  />
                  <EditInput
                    label={t('environment.healthRetries')}
                    type="number"
                    value={editData.healthcheck.retries}
                    onChange={(v) => handleHealthcheckChange('retries', parseInt(v) || 3)}
                  />
                  <EditInput
                    label={t('environment.healthStartPeriod')}
                    type="number"
                    value={editData.healthcheck.startPeriod}
                    onChange={(v) => handleHealthcheckChange('startPeriod', parseInt(v) || 0)}
                  />
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
