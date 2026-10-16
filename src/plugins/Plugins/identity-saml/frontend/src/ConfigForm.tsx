// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// identity-saml ConfigForm — 4 fields matching the plugin
// backend's ConfigSchema: metadata_url, entity_id, sp_certificate,
// sp_private_key. Fully controlled (value + onChange from the host)
// so the PluginFormOutlet's update() path doesn't lose keystrokes.

import { useTranslation } from 'react-i18next';

type FieldKey = 'metadata_url' | 'entity_id' | 'sp_certificate' | 'sp_private_key';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, message: string | null) => void;
  readOnlyFields?: string[];
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

const FIELD_META: Record<FieldKey, { required: boolean; secret: boolean; placeholder: string }> = {
  metadata_url: {
    required: true,
    secret: false,
    placeholder: 'https://idp.example.com/saml/metadata',
  },
  entity_id: {
    required: false,
    secret: false,
    placeholder: 'https://mcharbor.example.com',
  },
  sp_certificate: {
    required: false,
    secret: true,
    placeholder: '-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----',
  },
  sp_private_key: {
    required: false,
    secret: true,
    placeholder: '-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----',
  },
};

const FIELDS: FieldKey[] = ['metadata_url', 'entity_id', 'sp_certificate', 'sp_private_key'];

export function ConfigForm({
  value,
  onChange,
  onError: _onError,
  readOnlyFields: _readOnlyFields = [],
  t: externalT,
}: Props) {
  const localT = useTranslation('identity-saml').t;
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
            {key === 'metadata_url' || key === 'entity_id' ? (
              <input
                type="text"
                value={value[key] ?? ''}
                onChange={(e) => update(key, e.target.value)}
                placeholder={meta.placeholder}
                autoComplete="off"
                spellCheck={false}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/40 focus:outline-none"
              />
            ) : (
              <textarea
                value={value[key] ?? ''}
                onChange={(e) => update(key, e.target.value)}
                placeholder={meta.placeholder}
                autoComplete="off"
                spellCheck={false}
                rows={5}
                className="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/40 focus:outline-none"
              />
            )}
            <span className="block text-xs text-muted-foreground">{t(`help.${key}`)}</span>
          </label>
        );
      })}
    </div>
  );
}
