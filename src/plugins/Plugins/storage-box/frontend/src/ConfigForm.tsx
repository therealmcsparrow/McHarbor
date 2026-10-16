// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ConfigForm — Box destination row in the storage-location editor.
//
// This is the *plugin-supplied* form. The host also ships a
// default renderer driven by ConfigFieldDecl from
// backend/main.go (so the form works even before the bundle
// has loaded). The host mounts this bundle when the
// operator selects the Box destination type.
//
// Field list mirrors boxConfigSchema() in backend/main.go so
// the host's default renderer produces the same labels.
// Field shape is intentionally minimal: client_id +
// client_secret + folder_id + (optional) refresh_token
// shown only after the OAuth connect flow.
//
// Strings come from the host-supplied `t` (namespace = plugin slug); the
// bundle has no i18next instance of its own, so useTranslation is unavailable.

export interface ConfigFormProps {
  value: Record<string, string>;
  onChange?: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
}

interface Field {
  key: string;
  labelKey: string;
  type: 'string' | 'secret' | 'number';
  placeholder?: string;
  helpKey?: string;
  required?: boolean;
  readOnly?: boolean;
}

const FIELDS: Field[] = [
  {
    key: 'client_id',
    labelKey: 'fields.clientId',
    type: 'string',
    placeholder: 'cli_xxxxxxxxxxxx',
    helpKey: 'form.help.clientId',
    required: true,
  },
  {
    key: 'client_secret',
    labelKey: 'fields.clientSecret',
    type: 'secret',
    placeholder: '••••••••••••••••',
    helpKey: 'form.help.clientSecret',
    required: true,
  },
  {
    key: 'folder_id',
    labelKey: 'fields.folderId',
    type: 'string',
    placeholder: '0',
    helpKey: 'form.help.folderId',
  },
  {
    key: 'refresh_token',
    labelKey: 'fields.refreshToken',
    type: 'secret',
    placeholder: '••••••••••••••••',
    helpKey: 'form.help.refreshToken',
    readOnly: true,
  },
];

export function ConfigForm({ value, onChange, t: hostT }: ConfigFormProps) {
  const t = (key: string, vars?: Record<string, unknown>) => hostT?.(key, vars) ?? key;

  const setField = (key: string, v: string) => {
    onChange?.({ ...value, [key]: v });
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
        {FIELDS.map((f) => {
          const v = value[f.key] ?? '';
          const isReadOnly = !!f.readOnly && v !== '';
          return (
            <div key={f.key} className="space-y-1.5">
              <label htmlFor={`storage-box-${f.key}`} className="text-sm font-medium text-foreground">
                {t(f.labelKey)}
                {f.required && <span className="ml-1 text-destructive">*</span>}
              </label>
              <input
                id={`storage-box-${f.key}`}
                type={f.type === 'secret' ? 'password' : 'text'}
                autoComplete="off"
                spellCheck={false}
                value={v}
                placeholder={f.placeholder ?? ''}
                readOnly={isReadOnly}
                onChange={(e) => setField(f.key, e.target.value)}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50"
              />
              {f.helpKey && <p className="text-xs text-muted-foreground">{t(f.helpKey)}</p>}
            </div>
          );
        })}
      </div>

      <div className="rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground">
        {t('help.oauthHint')}
      </div>
    </div>
  );
}

export default ConfigForm;
