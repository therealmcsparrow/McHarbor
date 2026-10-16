// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { IconKey, IconLogin2 } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { cn } from '@resources/utils/cn';
import type { AIProviderAuthMethod, AIProviderOAuthInput } from '../hooks/useAIProviderConnections';
import { RadioCard } from './RadioCard';
import { AIProviderOAuthConfigFields } from './AIProviderOAuthConfigFields';

export type AIProviderAuthDraft = {
  authMethod: AIProviderAuthMethod;
  apiKey: string;
  oauth: AIProviderOAuthInput;
};

export const emptyOAuthDraft: AIProviderOAuthInput = {
  clientId: '',
  clientSecret: '',
  deviceUrl: '',
  tokenUrl: '',
  scopes: '',
};

type AIProviderAuthFieldsProps = {
  value: AIProviderAuthDraft;
  /** Patch-shaped rather than key/value: the same handler has to serve
   *  the create dialog (one draft object) and the edit dialog (three
   *  separate state hooks), and a generic key/value setter cannot be
   *  narrowed to a subset of a wider draft without a cast. */
  onChange: (patch: Partial<AIProviderAuthDraft>) => void;
  /** Prefix for input ids — the create and edit dialogs render these
   *  same fields, and two matching `htmlFor`s would break both. */
  idPrefix: string;
  /** Shown in the API key box when a key is already stored. */
  apiKeyPlaceholder?: string;
  /** Explains the "leave blank to keep" rule in the edit dialog. */
  apiKeyHelp?: string;
  /** A client secret is already stored for this connection. */
  hasClientSecret?: boolean;
  /** The provider's plugin ships a complete OAuth config, so the
   *  operator enters nothing: the editable config fields are replaced
   *  by a note and Sign in works immediately. */
  oauthPreconfigured?: boolean;
};

/**
 * How this connection authenticates: a pasted API key, or a sign-in.
 *
 * Both stay available per connection — plenty of providers only issue
 * keys, and plenty of operators would rather not have a long-lived key
 * sitting in a database. The sign-in path is the OAuth 2.0 device flow
 * (RFC 8628), the only grant that works for a self-hosted install with
 * no publicly reachable redirect URL: McHarbor shows a short code, the
 * operator approves it on the provider's own site.
 *
 * McHarbor registers no OAuth application of its own, so the client ID
 * is always the operator's. Known providers get their two endpoint URLs
 * prefilled by the caller; anything else is typed in here.
 */
export function AIProviderAuthFields({
  value,
  onChange,
  idPrefix,
  apiKeyPlaceholder,
  apiKeyHelp,
  hasClientSecret,
  oauthPreconfigured,
}: AIProviderAuthFieldsProps) {
  const { t } = useTranslation('settings');
  const groupName = useId();
  const isOAuth = value.authMethod === 'oauth';

  function setOAuth(key: keyof AIProviderOAuthInput, next: string) {
    onChange({ oauth: { ...value.oauth, [key]: next } });
  }

  const methods: Array<{
    method: AIProviderAuthMethod;
    label: string;
    hint: string;
    Icon: typeof IconKey;
  }> = [
    {
      method: 'api_key',
      label: t('aiProviders.auth.apiKeyOption'),
      hint: t('aiProviders.auth.apiKeyHint'),
      Icon: IconKey,
    },
    {
      method: 'oauth',
      label: t('aiProviders.auth.oauthOption'),
      hint: t('aiProviders.auth.oauthHint'),
      Icon: IconLogin2,
    },
  ];

  return (
    <div className="space-y-4">
      <div>
        <p className="text-xs font-medium text-foreground">{t('aiProviders.auth.method')}</p>
        <div role="radiogroup" aria-label={t('aiProviders.auth.method')} className="mt-1 flex flex-wrap gap-2">
          {methods.map(({ method, label, hint, Icon }) => {
            const selected = value.authMethod === method;
            return (
              <RadioCard
                key={method}
                name={groupName}
                checked={selected}
                onSelect={() => onChange({ authMethod: method })}
                className="min-w-[180px] flex-1 items-start justify-start p-3 text-left"
              >
                <span className="flex items-start gap-2">
                  <Icon className={cn('mt-0.5 size-4 shrink-0', selected ? 'text-primary' : 'text-muted-foreground')} />
                  <span className="flex flex-col gap-0.5">
                    <span className="text-sm font-medium text-foreground">{label}</span>
                    <span className="text-[11px] font-normal text-muted-foreground">{hint}</span>
                  </span>
                </span>
              </RadioCard>
            );
          })}
        </div>
      </div>

      {!isOAuth && (
        <div>
          <label htmlFor={`${idPrefix}-api-key`} className="text-xs font-medium text-foreground">
            {t('aiProviders.fields.apiKeyCreate')}
          </label>
          <Input
            id={`${idPrefix}-api-key`}
            type="password"
            value={value.apiKey}
            onChange={(e) => onChange({ apiKey: e.target.value })}
            placeholder={apiKeyPlaceholder ?? t('aiProviders.placeholders.apiKey')}
            className="mt-1"
          />
          {apiKeyHelp && <p className="mt-1 text-[11px] text-muted-foreground">{apiKeyHelp}</p>}
        </div>
      )}

      {isOAuth && oauthPreconfigured && (
        <div className="rounded-md border border-border bg-background p-3">
          <p className="text-[11px] text-muted-foreground">{t('aiProviders.auth.oauthPreconfigured')}</p>
        </div>
      )}

      {isOAuth && !oauthPreconfigured && (
        <AIProviderOAuthConfigFields
          value={value.oauth}
          onChange={setOAuth}
          idPrefix={idPrefix}
          hasClientSecret={hasClientSecret}
        />
      )}
    </div>
  );
}
