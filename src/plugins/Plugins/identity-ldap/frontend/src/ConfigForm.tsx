// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// identity-ldap ConfigForm — 13 fields matching the plugin backend's
// ConfigSchema. Fields in vertical order:
//   1. Server URL
//   2. Bind DN + password
//   3. User Base DN
//   4. User search (filter + attr)
//   5. Claim attrs (email + name)
//   6. TLS (StartTLS + skip-verify toggle)
//   7. Group Base DN + filter + attr
//
// Fully controlled (value + onChange from the host) so the
// PluginFormOutlet's update() path doesn't lose keystrokes.

import { useTranslation } from 'react-i18next';

type FieldKey =
  | 'url'
  | 'bind_dn'
  | 'bind_password'
  | 'user_base_dn'
  | 'user_filter'
  | 'user_attr'
  | 'email_attr'
  | 'name_attr'
  | 'start_tls'
  | 'skip_tls_verify'
  | 'group_base_dn'
  | 'group_filter'
  | 'group_attr';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, message: string | null) => void;
  readOnlyFields?: string[];
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

const FIELD_META: Record<FieldKey, { required: boolean; secret: boolean; toggle: boolean; placeholder: string }> = {
  url: {
    required: true,
    secret: false,
    toggle: false,
    placeholder: 'ldaps://ldap.example.com:636',
  },
  bind_dn: {
    required: false,
    secret: false,
    toggle: false,
    placeholder: 'cn=mcharbor,ou=service,dc=example,dc=com',
  },
  bind_password: {
    required: false,
    secret: true,
    toggle: false,
    placeholder: '',
  },
  user_base_dn: {
    required: true,
    secret: false,
    toggle: false,
    placeholder: 'ou=users,dc=example,dc=com',
  },
  user_filter: {
    required: false,
    secret: false,
    toggle: false,
    placeholder: '(uid=%s)',
  },
  user_attr: {
    required: false,
    secret: false,
    toggle: false,
    placeholder: 'uid',
  },
  email_attr: {
    required: false,
    secret: false,
    toggle: false,
    placeholder: 'mail',
  },
  name_attr: {
    required: false,
    secret: false,
    toggle: false,
    placeholder: 'cn',
  },
  start_tls: {
    required: false,
    secret: false,
    toggle: true,
    placeholder: '',
  },
  skip_tls_verify: {
    required: false,
    secret: false,
    toggle: true,
    placeholder: '',
  },
  group_base_dn: {
    required: false,
    secret: false,
    toggle: false,
    placeholder: 'ou=groups,dc=example,dc=com',
  },
  group_filter: {
    required: false,
    secret: false,
    toggle: false,
    placeholder: '(|(member=%s)(uniqueMember=%s))',
  },
  group_attr: {
    required: false,
    secret: false,
    toggle: false,
    placeholder: 'cn',
  },
};

const FIELDS: FieldKey[] = [
  'url',
  'bind_dn',
  'bind_password',
  'user_base_dn',
  'user_filter',
  'user_attr',
  'email_attr',
  'name_attr',
  'start_tls',
  'skip_tls_verify',
  'group_base_dn',
  'group_filter',
  'group_attr',
];

export function ConfigForm({
  value,
  onChange,
  onError: _onError,
  readOnlyFields: _readOnlyFields = [],
  t: externalT,
}: Props) {
  const localT = useTranslation('identity-ldap').t;
  const t = externalT ?? localT;

  function update(key: FieldKey, v: string) {
    onChange({ ...value, [key]: v });
  }

  return (
    <div className="space-y-4">
      {FIELDS.map((key) => {
        const meta = FIELD_META[key];
        const isSecret = meta.secret;
        const isToggle = meta.toggle;
        return (
          <label key={key} className="block space-y-1">
            <span className="text-sm font-medium text-foreground">
              {t(`fields.${key}`)}
              {meta.required ? <span className="ml-1 text-destructive">*</span> : null}
            </span>
            {isToggle ? (
              <div className="flex items-center gap-3">
                <input
                  type="checkbox"
                  checked={value[key] === 'true'}
                  onChange={(e) => update(key, e.target.checked ? 'true' : 'false')}
                  className="size-4 rounded border-input accent-primary focus:ring-2 focus:ring-ring/40"
                />
                <span className="text-xs text-muted-foreground">{t(`help.${key}`)}</span>
              </div>
            ) : isSecret ? (
              <input
                type="password"
                value={value[key] ?? ''}
                onChange={(e) => update(key, e.target.value)}
                placeholder={meta.placeholder}
                autoComplete="off"
                spellCheck={false}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/40 focus:outline-none"
              />
            ) : (
              <input
                type="text"
                value={value[key] ?? ''}
                onChange={(e) => update(key, e.target.value)}
                placeholder={meta.placeholder}
                autoComplete="off"
                spellCheck={false}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/40 focus:outline-none"
              />
            )}
            {!isToggle ? <span className="block text-xs text-muted-foreground">{t(`help.${key}`)}</span> : null}
          </label>
        );
      })}
    </div>
  );
}
