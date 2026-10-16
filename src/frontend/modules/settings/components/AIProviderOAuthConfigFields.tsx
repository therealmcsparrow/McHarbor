// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import type { AIProviderOAuthInput } from '../hooks/useAIProviderConnections';

type AIProviderOAuthConfigFieldsProps = {
  value: AIProviderOAuthInput;
  onChange: (key: keyof AIProviderOAuthInput, next: string) => void;
  idPrefix: string;
  /** A client secret is already stored — the field then means
   *  "leave blank to keep it", like the API key box. */
  hasClientSecret?: boolean;
};

/**
 * The device-flow endpoints a sign-in connection needs.
 *
 * These are the operator's own OAuth application: McHarbor ships no
 * registered client, so the client ID is always theirs. For providers
 * with published endpoints the two URLs arrive prefilled; everything
 * else is typed here, which is what keeps the flow usable for provider
 * types that arrive later as plugins.
 */
export function AIProviderOAuthConfigFields({
  value,
  onChange,
  idPrefix,
  hasClientSecret,
}: AIProviderOAuthConfigFieldsProps) {
  const { t } = useTranslation('settings');

  return (
    <div className="space-y-3 rounded-md border border-border bg-background p-3">
      <p className="text-[11px] text-muted-foreground">{t('aiProviders.auth.oauthSetupHelp')}</p>
      <OAuthField
        id={`${idPrefix}-oauth-client-id`}
        label={t('aiProviders.fields.oauthClientId')}
        value={value.clientId}
        onChange={(next) => onChange('clientId', next)}
        placeholder={t('aiProviders.placeholders.oauthClientId')}
      />
      <OAuthField
        id={`${idPrefix}-oauth-client-secret`}
        label={t('aiProviders.fields.oauthClientSecret')}
        value={value.clientSecret}
        onChange={(next) => onChange('clientSecret', next)}
        placeholder={hasClientSecret ? '••••••••' : t('aiProviders.placeholders.oauthClientSecret')}
        type="password"
        help={t('aiProviders.help.oauthClientSecret')}
      />
      <OAuthField
        id={`${idPrefix}-oauth-device-url`}
        label={t('aiProviders.fields.oauthDeviceUrl')}
        value={value.deviceUrl}
        onChange={(next) => onChange('deviceUrl', next)}
        placeholder={t('aiProviders.placeholders.oauthDeviceUrl')}
      />
      <OAuthField
        id={`${idPrefix}-oauth-token-url`}
        label={t('aiProviders.fields.oauthTokenUrl')}
        value={value.tokenUrl}
        onChange={(next) => onChange('tokenUrl', next)}
        placeholder={t('aiProviders.placeholders.oauthTokenUrl')}
      />
      <OAuthField
        id={`${idPrefix}-oauth-scopes`}
        label={t('aiProviders.fields.oauthScopes')}
        value={value.scopes}
        onChange={(next) => onChange('scopes', next)}
        placeholder={t('aiProviders.placeholders.oauthScopes')}
      />
    </div>
  );
}

type OAuthFieldProps = {
  id: string;
  label: string;
  value: string;
  onChange: (next: string) => void;
  placeholder?: string;
  type?: string;
  help?: string;
};

function OAuthField({ id, label, value, onChange, placeholder, type, help }: OAuthFieldProps) {
  return (
    <div>
      <label htmlFor={id} className="text-xs font-medium text-foreground">
        {label}
      </label>
      <Input
        id={id}
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="mt-1"
      />
      {help && <p className="mt-1 text-[11px] text-muted-foreground">{help}</p>}
    </div>
  );
}
