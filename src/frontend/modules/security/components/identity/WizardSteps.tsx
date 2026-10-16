// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconHelpCircle, IconPuzzle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { PluginFormOutlet } from '@core/plugins/PluginFormOutlet';
import { SetupGuidePanel } from '@resources/components/SetupGuidePanel';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';
import type { IdentityAdapterSpec } from '@resources/hooks/useIdentityAdapters';
import { MetadataImportPanel } from './MetadataImportPanel';
import { ProviderEndpointUrls } from './ProviderEndpointUrls';
import type { ProviderWizard } from './useProviderWizard';

const EMPTY: Record<string, string> = {};

export function WizardTypeStep({
  adapters,
  isLoading,
  onSelect,
}: {
  adapters: IdentityAdapterSpec[];
  isLoading: boolean;
  onSelect: (type: string) => void;
}) {
  const { t } = useTranslation('security');
  if (isLoading)
    return <div className="py-8 text-center text-sm text-muted-foreground">{t('common:common.loading')}</div>;
  if (adapters.length === 0)
    return <div className="py-8 text-center text-sm text-muted-foreground">{t('identity.noAdaptersInstalled')}</div>;
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {adapters.map((spec) => (
        <Button
          key={spec.ProviderType}
          variant="outline"
          onClick={() => onSelect(spec.ProviderType)}
          className="flex h-auto flex-col items-center gap-3 p-6"
          aria-label={spec.DisplayName}
        >
          <div className="size-10 text-primary">
            {spec.Slug && spec.Icon ? (
              <PluginAssetImage
                slug={spec.Slug}
                icon={spec.Icon}
                alt={spec.DisplayName}
                className="size-full object-contain"
              />
            ) : (
              <IconPuzzle aria-hidden="true" />
            )}
          </div>
          <span className="text-center text-xs font-medium break-words whitespace-normal text-foreground">
            {spec.DisplayName}
          </span>
        </Button>
      ))}
    </div>
  );
}

export function WizardConfigureStep({ wizard, spec }: { wizard: ProviderWizard; spec: IdentityAdapterSpec | null }) {
  const { t } = useTranslation('security');
  const [helpOpen, setHelpOpen] = useState(false);
  const type = wizard.providerType ?? '';
  const saml = Boolean(spec?.Capabilities.SAMLRedirect || spec?.Capabilities.SupportsMetadata);
  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="create-provider-name" className="mb-1">
          {t('identity.name')}
          <span className="ml-1 text-destructive">*</span>
        </Label>
        <Input
          variant="outline"
          id="create-provider-name"
          value={wizard.config.name ?? ''}
          onChange={(e) => wizard.setConfig((current) => ({ ...current, name: e.target.value }))}
          placeholder={t('identity.namePlaceholder')}
          autoComplete="off"
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('identity.nameDescription')}</p>
      </div>
      {saml && <MetadataImportPanel schema={spec?.ConfigSchema} onPrefill={wizard.prefill} />}
      <PluginFormOutlet
        resource="identity_provider"
        kind="config_form"
        keyName={type}
        value={wizard.config}
        onChange={wizard.setConfig}
        i18nPrefix="plugin"
      />
      <div>
        <p className="mb-1 text-sm font-medium text-foreground">{t('endpoints.title', { ns: 'access' })}</p>
        <ProviderEndpointUrls capabilities={spec?.Capabilities} />
      </div>
      <Button variant="ghost" size="sm" onClick={() => setHelpOpen((isOpen) => !isOpen)} aria-expanded={helpOpen}>
        <IconHelpCircle className="size-4" />
        {helpOpen ? t('identity.hideSetupGuide') : t('identity.showSetupGuide')}
      </Button>
      {helpOpen && (
        <SetupGuidePanel>
          <PluginFormOutlet
            resource="identity_provider"
            kind="setup_guide"
            keyName={type}
            value={EMPTY}
            onChange={() => undefined}
            i18nPrefix="plugin"
          />
        </SetupGuidePanel>
      )}
    </div>
  );
}
