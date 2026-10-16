// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ConfigForm renders the per-destination config UI for the
// storage-nfs plugin. Fields mirror backend/main.go
// nfsConfigSchema(); the field types and options must stay
// in sync with that schema.
//
// Conditional fields:
//   - sec=sys       hides principal, realm, keytab_path
//   - sec=krb5/...  shows principal, realm, keytab_path
//   - mount_base    always visible (advanced; default kept)
//
// The plugin does not call react-i18next's useTranslation (the
// bundle has no i18next instance); strings come from the
// host-supplied `t`, in the namespace named after the plugin slug.

import { IconServer, IconLock, IconFolder, IconKey } from '@tabler/icons-react';

export interface ConfigFormProps {
  value: Record<string, string>;
  errors?: Record<string, string>;
  onChange?: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
}

interface FieldDef {
  key: string;
  type: 'text' | 'password' | 'select';
  required: boolean;
  // Predicate: when defined, only render the field if it
  // returns true. Used to hide the Kerberos fields when the
  // operator chose sec=sys.
  visible?: (value: Record<string, string>) => boolean;
  options?: string[];
  icon?: React.ComponentType<{ className?: string }>;
  labelKey: string;
  placeholder?: string;
  helpKey?: string;
  // Extra help shown after the input. Used for the keytab
  // warning that the host process must have read access.
  helpTextKey?: string;
}

const FIELDS: FieldDef[] = [
  {
    key: 'server',
    type: 'text',
    required: true,
    icon: IconServer,
    labelKey: 'storage-nfs.config.field.server.label',
    placeholder: 'nfs.example.com or 10.0.0.5',
    helpKey: 'storage-nfs.config.field.server.help',
  },
  {
    key: 'export',
    type: 'text',
    required: true,
    icon: IconFolder,
    labelKey: 'storage-nfs.config.field.export.label',
    placeholder: '/srv/backups',
    helpKey: 'storage-nfs.config.field.export.help',
  },
  {
    key: 'path',
    type: 'text',
    required: false,
    icon: IconFolder,
    labelKey: 'storage-nfs.config.field.path.label',
    placeholder: 'mcharbor',
    helpKey: 'storage-nfs.config.field.path.help',
  },
  {
    key: 'sec',
    type: 'select',
    required: true,
    options: ['sys', 'krb5', 'krb5i', 'krb5p'],
    icon: IconLock,
    labelKey: 'storage-nfs.config.field.sec.label',
    helpKey: 'storage-nfs.form.field.sec.help',
  },
  {
    key: 'version',
    type: 'select',
    required: false,
    options: ['auto', '3', '4', '4.1', '4.2'],
    labelKey: 'storage-nfs.config.field.version.label',
    helpKey: 'storage-nfs.config.field.version.help',
  },
  {
    key: 'principal',
    type: 'text',
    required: false,
    visible: (v) => v.sec !== 'sys' && v.sec !== undefined && v.sec !== '',
    icon: IconKey,
    labelKey: 'storage-nfs.config.field.principal.label',
    placeholder: 'nfs/host.nfs.example.com',
    helpKey: 'storage-nfs.form.field.principal.help',
  },
  {
    key: 'realm',
    type: 'text',
    required: false,
    visible: (v) => v.sec !== 'sys' && v.sec !== undefined && v.sec !== '',
    icon: IconKey,
    labelKey: 'storage-nfs.config.field.realm.label',
    placeholder: 'EXAMPLE.COM',
    helpKey: 'storage-nfs.form.field.realm.help',
  },
  {
    key: 'keytab_path',
    type: 'text',
    required: false,
    visible: (v) => v.sec !== 'sys' && v.sec !== undefined && v.sec !== '',
    icon: IconKey,
    labelKey: 'storage-nfs.config.field.keytab_path.label',
    placeholder: '/etc/krb5.keytab',
    helpKey: 'storage-nfs.config.field.keytab_path.help',
    helpTextKey: 'storage-nfs.form.field.keytab_path.helpText',
  },
  {
    key: 'mount_base',
    type: 'text',
    required: false,
    icon: IconFolder,
    labelKey: 'storage-nfs.config.field.mount_base.label',
    placeholder: '/var/lib/mcharbor/mounts/nfs',
    helpKey: 'storage-nfs.config.field.mount_base.help',
  },
];

const HEADING_KEY = 'storage-nfs.config.heading';
const DESCRIPTION_KEY = 'storage-nfs.config.description';
const REQUIRED_KEY = 'storage-nfs.form.required';

export function ConfigForm({ value, errors, onChange, t: hostT }: ConfigFormProps) {
  const t = (key: string, vars?: Record<string, unknown>) => hostT?.(key, vars) ?? key;

  // The visible predicate reads value.sec, so we render a no-op
  // span that depends on sec to force the predicate to
  // re-evaluate when sec changes.
  const sec = value.sec ?? 'sys';

  const setField = (key: string, v: string) => {
    onChange?.({ ...value, [key]: v });
  };

  const visibleFields = FIELDS.filter((f) => (f.visible ? f.visible(value) : true));

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-semibold text-foreground">{t(HEADING_KEY)}</h3>
        <p className="mt-1 text-xs text-muted-foreground">{t(DESCRIPTION_KEY)}</p>
      </div>

      {visibleFields.map((field) => {
        const v = value[field.key] ?? '';
        const error = errors?.[field.key];
        const Icon = field.icon;

        return (
          <div key={field.key} className="space-y-1.5">
            <label htmlFor={`nfs-${field.key}`} className="flex items-center gap-2 text-xs font-medium text-foreground">
              {Icon && <Icon className="h-3.5 w-3.5 text-muted-foreground" />}
              <span>{t(field.labelKey)}</span>
              {field.required && (
                <span className="text-xs text-destructive" aria-label={t(REQUIRED_KEY)}>
                  *
                </span>
              )}
            </label>

            {field.type === 'select' ? (
              <select
                id={`nfs-${field.key}`}
                value={v}
                onChange={(e) => setField(field.key, e.target.value)}
                className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground focus:ring-2 focus:ring-ring focus:outline-none"
              >
                {field.options?.map((opt) => (
                  <option key={opt} value={opt}>
                    {opt}
                  </option>
                ))}
              </select>
            ) : (
              <input
                id={`nfs-${field.key}`}
                type={field.type === 'password' ? 'password' : 'text'}
                value={v}
                onChange={(e) => setField(field.key, e.target.value)}
                placeholder={field.placeholder}
                aria-invalid={error ? 'true' : 'false'}
                aria-describedby={field.helpKey ? `nfs-${field.key}-help` : undefined}
                className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:ring-2 focus:ring-ring focus:outline-none"
              />
            )}

            {field.helpKey && (
              <p id={`nfs-${field.key}-help`} className="text-xs text-muted-foreground">
                {t(field.helpKey)}
              </p>
            )}
            {field.helpTextKey && <p className="text-xs text-muted-foreground">{t(field.helpTextKey)}</p>}
            {error && (
              <p className="text-xs text-destructive" role="alert" aria-live="polite">
                {error}
              </p>
            )}
          </div>
        );
      })}

      {/* Hidden helper: forces visibleFields re-eval when sec changes. */}
      <span aria-hidden="true" className="hidden" data-current-sec={sec} />
    </div>
  );
}

export default ConfigForm;
