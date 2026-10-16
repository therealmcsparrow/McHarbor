// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// identity-generic-oidc ConfigForm — four fields matching the plugin
// backend's ConfigSchema: issuer_url, client_id, client_secret,
// scopes. Fully controlled (value + onChange from the host) so the
// PluginFormOutlet's update() path doesn't lose keystrokes the way
// a useState-driven version would.

import { useTranslation } from 'react-i18next';

type FieldKey = 'issuer_url' | 'client_id' | 'client_secret' | 'scopes';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, message: string | null) => void;
  readOnlyFields?: string[];
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

const FIELD_META: Record<FieldKey, { required: boolean; secret: boolean; placeholder: string }> = {
  issuer_url: {
    required: true,
    secret: false,
    placeholder: 'https://login.example.com',
  },
  client_id: {
    required: true,
    secret: false,
    placeholder: 'mcharbor-client',
  },
  client_secret: { required: true, secret: true, placeholder: '' },
  scopes: {
    required: false,
    secret: false,
    placeholder: 'openid profile email groups',
  },
};

// Field order mirrors the host's pre-existing GenericOIDCForm.tsx
// layout so operators switching from the old host form see fields in
// the same vertical order.
const FIELDS: FieldKey[] = ['issuer_url', 'client_id', 'client_secret', 'scopes'];

export function ConfigForm({
  value,
  onChange,
  onError: _onError,
  readOnlyFields: _readOnlyFields = [],
  t: externalT,
}: Props) {
  const localT = useTranslation('identity-generic-oidc').t;
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
