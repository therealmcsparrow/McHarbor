// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// identity-google ConfigForm — 8 fields matching the plugin
// backend's ConfigSchema. The first four are required for the
// basic OAuth login flow; the remaining four are optional and
// enable Admin SDK group fetching. Fully controlled (value +
// onChange from the host) so the PluginFormOutlet's update()
// path doesn't lose keystrokes.

import { useTranslation } from 'react-i18next';

type FieldKey =
  | 'client_id'
  | 'client_secret'
  | 'domain'
  | 'scopes'
  | 'service_account_json'
  | 'delegated_user_email'
  | 'delegated_user_access_token'
  | 'user_email_for_lookup';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, message: string | null) => void;
  readOnlyFields?: string[];
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

const FIELD_META: Record<FieldKey, { required: boolean; secret: boolean; placeholder: string }> = {
  client_id: {
    required: true,
    secret: false,
    placeholder: '000000000000-aaaaaaaaaaaa.apps.googleusercontent.com',
  },
  client_secret: { required: true, secret: true, placeholder: '' },
  domain: {
    required: false,
    secret: false,
    placeholder: 'example.com',
  },
  scopes: {
    required: false,
    secret: false,
    placeholder: 'openid profile email',
  },
  service_account_json: {
    required: false,
    secret: true,
    placeholder: '{ "type": "service_account", ... }',
  },
  delegated_user_email: {
    required: false,
    secret: false,
    placeholder: 'admin@example.com',
  },
  delegated_user_access_token: {
    required: false,
    secret: true,
    placeholder: 'ya29.a0AcM...',
  },
  user_email_for_lookup: {
    required: false,
    secret: false,
    placeholder: 'user@example.com',
  },
};

const FIELDS: FieldKey[] = [
  'client_id',
  'client_secret',
  'domain',
  'scopes',
  'service_account_json',
  'delegated_user_email',
  'delegated_user_access_token',
  'user_email_for_lookup',
];

export function ConfigForm({
  value,
  onChange,
  onError: _onError,
  readOnlyFields: _readOnlyFields = [],
  t: externalT,
}: Props) {
  const localT = useTranslation('identity-google').t;
  const t = externalT ?? localT;

  function update(key: FieldKey, v: string) {
    onChange({ ...value, [key]: v });
  }

  return (
    <div className="space-y-4">
      {FIELDS.map((key) => {
        const meta = FIELD_META[key];
        return (
          <label key={key} className="block space-y-1">
            <span className="text-sm font-medium text-foreground">
              {t(`fields.${key}`)}
              {meta.required ? <span className="ml-1 text-destructive">*</span> : null}
            </span>
            <input
              type={meta.secret ? 'password' : 'text'}
              value={value[key] ?? ''}
              onChange={(e) => update(key, e.target.value)}
              placeholder={meta.placeholder}
              autoComplete="off"
              spellCheck={false}
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/40 focus:outline-none"
            />
            <span className="block text-xs text-muted-foreground">{t(`help.${key}`)}</span>
          </label>
        );
      })}
    </div>
  );
}
