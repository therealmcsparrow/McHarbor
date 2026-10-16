// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { AIProviderAuthFields, type AIProviderAuthDraft } from './AIProviderAuthFields';

/** The credential half (`authMethod` / `apiKey` / `oauth`) lives in
 *  AIProviderAuthDraft so the create and edit dialogs share one shape
 *  for the part that decides how the connection authenticates. */
export type AIProviderConnectionDraft = AIProviderAuthDraft & {
  name: string;
  baseUrl: string;
  defaultModel: string;
  isDefault: boolean;
};

type AIProviderConnectionFieldsProps = {
  value: AIProviderConnectionDraft;
  onChange: (patch: Partial<AIProviderConnectionDraft>) => void;
  /** The selected provider's plugin ships a complete OAuth config, so
   *  the sign-in needs no operator entry — the config fields collapse
   *  to a note. */
  oauthPreconfigured?: boolean;
};

/**
 * Step two of adding a connection: the per-connection settings. Base URL and
 * model are optional — the adapter supplies its own defaults when they are
 * blank, which is what most operators want.
 *
 * The credential block is delegated to AIProviderAuthFields, which offers the
 * two ways to authenticate: a pasted API key, or an OAuth sign-in.
 */
export function AIProviderConnectionFields({ value, onChange, oauthPreconfigured }: AIProviderConnectionFieldsProps) {
  const { t } = useTranslation('settings');

  return (
    <div className="space-y-4">
      <div>
        <label htmlFor="new-connection-name" className="text-xs font-medium text-foreground">
          {t('aiProviders.fields.name')}
        </label>
        <Input
          id="new-connection-name"
          value={value.name}
          onChange={(e) => onChange({ name: e.target.value })}
          placeholder={t('aiProviders.placeholders.name')}
          className="mt-1"
        />
      </div>
      <div>
        <label htmlFor="new-connection-base-url" className="text-xs font-medium text-foreground">
          {t('aiProviders.fields.apiBaseUrl')}
        </label>
        <Input
          id="new-connection-base-url"
          value={value.baseUrl}
          onChange={(e) => onChange({ baseUrl: e.target.value })}
          placeholder={t('aiProviders.placeholders.apiBaseUrl')}
          className="mt-1"
        />
        <p className="mt-1 text-[11px] text-muted-foreground">{t('aiProviders.help.apiBaseUrl')}</p>
      </div>
      <div>
        <label htmlFor="new-connection-default-model" className="text-xs font-medium text-foreground">
          {t('aiProviders.fields.defaultModel')}
        </label>
        <Input
          id="new-connection-default-model"
          value={value.defaultModel}
          onChange={(e) => onChange({ defaultModel: e.target.value })}
          placeholder={t('aiProviders.placeholders.defaultModel')}
          className="mt-1"
        />
      </div>
      <AIProviderAuthFields
        value={value}
        onChange={onChange}
        idPrefix="ai-provider"
        oauthPreconfigured={oauthPreconfigured}
      />
      <div className="flex items-center justify-between rounded-md border border-border bg-background p-3">
        <div>
          <p className="text-sm font-medium text-foreground">{t('aiProviders.fields.isDefault')}</p>
        </div>
        <Switch
          checked={value.isDefault}
          onCheckedChange={(next) => onChange({ isDefault: next })}
          aria-label={t('aiProviders.fields.isDefault')}
        />
      </div>
    </div>
  );
}
