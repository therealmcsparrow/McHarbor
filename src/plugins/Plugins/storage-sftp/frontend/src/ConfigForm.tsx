// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ConfigForm — the per-field config UI for an SFTP storage
// destination. The host's storage location form renders the
// common fields (name, location_type, etc.); this component
// handles ONLY the SFTP-specific fields declared in the
// plugin's ConfigSchema (backend/main.go).
//
// Conditional rendering:
//   - auth_method = "password"             → username + password
//   - auth_method = "publickey"            → username + private_key_path + private_key_passphrase
//   - auth_method = "keyboard_interactive" → username + password (echoed to server challenges)
//   - auth_method = "agent"                → username + agent_socket
//
// Server, port, host_key_fingerprint, path, and connect_timeout
// are always visible.
//
// UI primitives are inlined (native HTML + Tailwind) because
// the host's `@resources` alias does not resolve from a
// plugin's frontend build (see mcharbor-plugin-frontend-fix).

import * as React from 'react';
import { useTranslation } from 'react-i18next';
import { FIELDS, fieldIcon, type AuthMethod } from './fields';

// ─────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────

export type ConfigFormProps = {
  config: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  errors: Record<string, string> | null;
};

export function ConfigForm({ config, onChange, errors }: ConfigFormProps): React.ReactElement {
  const { t } = useTranslation('storage-sftp');
  const auth = (config.auth_method as AuthMethod) || 'password';

  function update(key: string, value: string): void {
    // For auth_method: clear the now-irrelevant credential fields
    // when switching modes, so the host never persists a stale
    // password alongside a publickey config.
    const next: Record<string, string> = { ...config, [key]: value };
    if (key === 'auth_method') {
      const droppingPassword = auth === 'password' || auth === 'keyboard_interactive';
      const droppingKey = auth === 'publickey';
      const droppingAgent = auth === 'agent';
      const nextAuth = value as AuthMethod;
      const nextDropsPassword = nextAuth !== 'password' && nextAuth !== 'keyboard_interactive';
      const nextDropsKey = nextAuth !== 'publickey';
      const nextDropsAgent = nextAuth !== 'agent';
      if (droppingPassword && nextDropsPassword) delete next.password;
      if (droppingKey && nextDropsKey) {
        delete next.private_key_path;
        delete next.private_key_passphrase;
      }
      if (droppingAgent && nextDropsAgent) delete next.agent_socket;
    }
    onChange(next);
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('config.intro')}</p>

      {FIELDS.filter((f) => !f.visible || f.visible(auth)).map((field) => {
        const value = config[field.key] ?? '';
        const error = errors?.[field.key];

        return (
          <div key={field.key} className="space-y-2">
            <label className="flex items-center gap-2 text-sm font-medium text-foreground">
              {fieldIcon(field.key)}
              {t(field.labelKey)}
            </label>

            {field.type === 'select' ? (
              <select
                value={value || auth}
                onChange={(e) => update(field.key, e.currentTarget.value)}
                aria-label={t(field.labelKey)}
                className="flex h-9 w-full rounded-lg border border-input bg-background px-3 py-1 text-sm shadow-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              >
                {(field.options ?? []).map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {t(opt.labelKey)}
                  </option>
                ))}
              </select>
            ) : field.type === 'secret' ? (
              <input
                type="password"
                value={value}
                onChange={(e) => update(field.key, e.currentTarget.value)}
                placeholder={field.placeholderKey ? t(field.placeholderKey) : undefined}
                aria-label={t(field.labelKey)}
                autoComplete="off"
                className="flex h-9 w-full rounded-lg border border-input bg-background px-3 py-1 text-sm shadow-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              />
            ) : (
              <input
                type="text"
                value={value}
                onChange={(e) => update(field.key, e.currentTarget.value)}
                placeholder={field.placeholderKey ? t(field.placeholderKey) : undefined}
                aria-label={t(field.labelKey)}
                autoComplete="off"
                className="flex h-9 w-full rounded-lg border border-input bg-background px-3 py-1 text-sm shadow-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              />
            )}

            <p className="text-sm text-muted-foreground">{t(field.helpKey)}</p>

            {error && <p className="text-sm text-destructive">{error}</p>}
          </div>
        );
      })}

      <div className="flex items-center justify-end gap-2 pt-2">
        {/* eslint-disable-next-line mcharbor/no-restricted-syntax -- this bundle has no @resources alias (see vite-plugin.config.ts), so Button/PlainButton cannot be imported */}
        <button
          type="button"
          onClick={() => onChange({})}
          aria-label={t('config.reset')}
          className="inline-flex h-9 items-center rounded-lg border border-border bg-background px-3 text-sm hover:bg-muted"
        >
          {t('config.reset')}
        </button>
      </div>
    </div>
  );
}

export default ConfigForm;
