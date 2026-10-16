// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ConfigForm renders the per-destination config UI for the
// storage-ftp plugin. Fields mirror backend/main.go
// ftpConfigSchema(); the field types and options must stay
// in sync with that schema.
//
// Conditional fields per auth_method:
//   - anonymous:    username + password + tls_skip_verify hidden
//   - password:     username + password
//   - ftps_explicit:username + password + tls_skip_verify
//   - ftps_implicit:username + password + tls_skip_verify
//   - client_cert:  client_cert_path + client_key_path
//
// All user-facing strings go through t('storage-ftp.*').
// See i18n/en/common.json for the source-of-truth keys.

import { useTranslation } from 'react-i18next';
import { FIELDS } from './fields';

const REQUIRED_LABEL = 'required';

export interface ConfigFormProps {
  cfg: Record<string, string>;
  errors?: Record<string, string>;
  onChange?: (cfg: Record<string, string>) => void;
  heading?: string;
  description?: string;
}

export default function ConfigForm({ cfg, errors, onChange, heading, description }: ConfigFormProps) {
  const { t } = useTranslation('storage-ftp');

  const authMethod = cfg.auth_method ?? 'password';

  const emit = (next: Record<string, string>) => {
    onChange?.(next);
  };

  const setField = (key: string, value: string) => {
    emit({ ...cfg, [key]: value });
  };

  const visibleFields = FIELDS.filter((f) => (f.visible ? f.visible(cfg) : true));

  return (
    <div className="space-y-4">
      {heading && (
        <div>
          <h3 className="text-sm font-semibold text-foreground">{heading}</h3>
          {description && <p className="mt-1 text-xs text-muted-foreground">{description}</p>}
        </div>
      )}

      {visibleFields.map((field) => {
        const value = cfg[field.key] ?? '';
        const error = errors?.[field.key];
        const Icon = field.icon;
        const label = t(field.labelKey);
        const help = field.helpKey ? t(field.helpKey) : undefined;
        const placeholder = field.placeholderKey ? t(field.placeholderKey) : undefined;

        return (
          <div key={field.key} className="space-y-1.5">
            <label htmlFor={`ftp-${field.key}`} className="flex items-center gap-2 text-xs font-medium text-foreground">
              {Icon && <Icon className="h-3.5 w-3.5 text-muted-foreground" />}
              <span>{label}</span>
              {field.required && (
                <span className="text-xs text-destructive" aria-label={REQUIRED_LABEL}>
                  *
                </span>
              )}
            </label>

            {field.type === 'select' ? (
              <select
                id={`ftp-${field.key}`}
                value={value}
                onChange={(e) => setField(field.key, e.target.value)}
                className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground focus:ring-2 focus:ring-ring focus:outline-none"
              >
                {field.options?.map((opt) => (
                  <option key={opt} value={opt}>
                    {t(`config.field.${field.key}.option.${opt}`)}
                  </option>
                ))}
              </select>
            ) : field.type === 'boolean' ? (
              <label htmlFor={`ftp-${field.key}`} className="flex items-center gap-2 text-sm text-foreground">
                <input
                  id={`ftp-${field.key}`}
                  type="checkbox"
                  checked={value === 'true'}
                  onChange={(e) => setField(field.key, e.target.checked ? 'true' : 'false')}
                  className="h-4 w-4 rounded border-input"
                />
                <span>{t('config.field.tls_skip_verify.value')}</span>
              </label>
            ) : (
              <input
                id={`ftp-${field.key}`}
                type={field.type === 'password' ? 'password' : 'text'}
                value={value}
                onChange={(e) => setField(field.key, e.target.value)}
                placeholder={placeholder}
                aria-invalid={error ? 'true' : 'false'}
                aria-describedby={help ? `ftp-${field.key}-help` : undefined}
                className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:ring-2 focus:ring-ring focus:outline-none"
              />
            )}

            {help && (
              <p id={`ftp-${field.key}-help`} className="text-xs text-muted-foreground">
                {help}
              </p>
            )}
            {error && (
              <p className="text-xs text-destructive" role="alert" aria-live="polite">
                {error}
              </p>
            )}
          </div>
        );
      })}

      {/* Hidden helper: the visible predicate reads cfg.auth_method,
          so we render a no-op span that depends on authMethod to
          force the predicate to re-evaluate when auth_method
          changes. */}
      <span aria-hidden="true" className="hidden" data-current-auth={authMethod} />
    </div>
  );
}
