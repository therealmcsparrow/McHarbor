// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ConfigForm — Samba / SMB destination row in the storage-location editor.
//
// This is the *plugin-supplied* form. The host also ships a default
// renderer driven by ConfigFieldDecl from backend/main.go (so the
// form works even before the bundle has loaded). The host mounts
// this bundle when the operator clicks the "Use plugin form"
// toggle in the destination editor.
//
// Auth-method-aware: the form re-renders its conditional fields
// (username/password for basic+domain, guest_user/guest_password
// for guest) as soon as the auth_method select changes — the
// host's default renderer does the same via field-level
// visibility hints, so operators get a consistent UX whether the
// bundle is loaded or not.

import React from 'react';
import { useTranslation } from 'react-i18next';
import type { PluginMountCtx } from './index';

interface ConfigFormProps {
  ctx: PluginMountCtx;
}

interface Field {
  key: string;
  label: string;
  type: 'string' | 'secret' | 'number';
  placeholder?: string;
  help?: string;
  required?: boolean;
  // visible is computed against ctx.cfg["auth_method"].
  visible?: (authMethod: string) => boolean;
}

// All fields except server/share/auth_method/path are conditional
// on the chosen auth_method. The conditional fields use the
// `visible` predicate to declare their visibility rule.
const FIELDS: Field[] = [
  {
    key: 'server',
    label: 'fields.server',
    type: 'string',
    placeholder: 'nas.local or 192.168.1.10',
    help: 'help.server',
    required: true,
  },
  {
    key: 'share',
    label: 'fields.share',
    type: 'string',
    placeholder: 'backups',
    help: 'help.share',
    required: true,
  },
  {
    key: 'path',
    label: 'fields.path',
    type: 'string',
    placeholder: '/',
    help: 'help.path',
  },
  {
    key: 'auth_method',
    label: 'fields.authMethod',
    type: 'string', // select — host default renderer uses Options
    placeholder: 'basic',
    help: 'help.authMethod',
    required: true,
  },
  {
    key: 'username',
    label: 'fields.username',
    type: 'string',
    placeholder: 'backup-user',
    help: 'help.username',
    required: true,
    visible: (m) => m === 'basic' || m === 'domain',
  },
  {
    key: 'password',
    label: 'fields.password',
    type: 'secret',
    placeholder: '••••••••••••••••',
    help: 'help.password',
    required: true,
    visible: (m) => m === 'basic' || m === 'domain',
  },
  {
    key: 'domain',
    label: 'fields.domain',
    type: 'string',
    placeholder: 'CORP',
    help: 'help.domain',
    required: true,
    visible: (m) => m === 'domain',
  },
  {
    key: 'guest_user',
    label: 'fields.guestUser',
    type: 'string',
    placeholder: 'nobody',
    help: 'help.guestUser',
    visible: (m) => m === 'guest',
  },
  {
    key: 'guest_password',
    label: 'fields.guestPassword',
    type: 'secret',
    placeholder: '••••••••••••••••',
    help: 'help.guestPassword',
    visible: (m) => m === 'guest',
  },
];

export function ConfigForm({ ctx }: ConfigFormProps) {
  const { t } = useTranslation('storage-samba');

  const authMethod = ctx.cfg['auth_method'] || 'basic';

  const onChange = (key: string, value: string) => {
    const next = { ...ctx.cfg, [key]: value };
    ctx.onChange?.(next);
  };

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold text-foreground">{t('page.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('page.subtitle')}</p>
      </header>

      <div className="rounded-lg border border-border bg-card p-4 text-xs text-muted-foreground">
        {t('page.registerHint')}
      </div>

      <div className="space-y-4">
        {FIELDS.filter((f) => !f.visible || f.visible(authMethod)).map((f) => {
          const value = ctx.cfg[f.key] ?? '';
          const isSelect = f.key === 'auth_method';
          return (
            <div key={f.key} className="space-y-1.5">
              <label htmlFor={`storage-samba-${f.key}`} className="text-sm font-medium text-foreground">
                {t(f.label)}
                {f.required && <span className="ml-1 text-destructive">*</span>}
              </label>
              {isSelect ? (
                <select
                  id={`storage-samba-${f.key}`}
                  value={value || 'basic'}
                  onChange={(e) => onChange(f.key, e.target.value)}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                >
                  <option value="guest">{t('auth.guest')}</option>
                  <option value="basic">{t('auth.basic')}</option>
                  <option value="domain">{t('auth.domain')}</option>
                </select>
              ) : (
                <input
                  id={`storage-samba-${f.key}`}
                  type={f.type === 'secret' ? 'password' : 'text'}
                  autoComplete="off"
                  spellCheck={false}
                  value={value}
                  placeholder={f.placeholder ?? ''}
                  onChange={(e) => onChange(f.key, e.target.value)}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50"
                />
              )}
              {f.help && <p className="text-xs text-muted-foreground">{t(f.help)}</p>}
            </div>
          );
        })}
      </div>
    </div>
  );
}
