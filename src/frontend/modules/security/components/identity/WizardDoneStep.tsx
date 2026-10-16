// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCircleCheck, IconPlugConnected } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import type { IdentityAdapterSpec } from '@resources/hooks/useIdentityAdapters';
import { PERM } from '@core/auth/permissionKeys';
import {
  useTestProvider,
  useUpdateProvider,
  type IdentityProvider as IdentityProviderValue,
} from '../../hooks/useIdentityProviders';
import { IdentityProviderTestDialog } from '../IdentityProviderTestDialog';
import { ProviderEndpointUrls } from './ProviderEndpointUrls';
import { MappingPreviewPanel } from './MappingPreviewPanel';
import type { MappingConfig } from '../../hooks/useIdentitySetup';

export function WizardDoneStep({
  provider,
  spec,
  mapping,
}: {
  provider: IdentityProviderValue;
  spec: IdentityAdapterSpec | null;
  mapping?: MappingConfig;
}) {
  const { t } = useTranslation('access');
  const [enabled, setEnabled] = useState(provider.enabled);
  const [testOpen, setTestOpen] = useState(false);
  const update = useUpdateProvider();
  const test = useTestProvider();

  const runTest = () => {
    setTestOpen(true);
    test.mutate(provider.id);
  };

  return (
    <div className="space-y-4">
      <Alert variant="success" title={t('wizard.done.title', { name: provider.name })}>
        {t('wizard.done.description')}
      </Alert>
      <ProviderEndpointUrls providerId={provider.id} capabilities={spec?.Capabilities} />
      <div className="flex flex-wrap items-center gap-2">
        {!enabled && (
          <Button
            variant="outline"
            permission={PERM.IDENTITY_PROVIDERS_MANAGE}
            disabled={update.isPending}
            onClick={() => update.mutate({ id: provider.id, enabled: true }, { onSuccess: () => setEnabled(true) })}
          >
            <IconCircleCheck className="size-4" />
            {t('wizard.done.enable')}
          </Button>
        )}
        <Button permission={PERM.IDENTITY_PROVIDERS_MANAGE} disabled={!enabled || test.isPending} onClick={runTest}>
          <IconPlugConnected className="size-4" />
          {t('wizard.done.test')}
        </Button>
      </div>
      {!enabled && <p className="text-xs text-muted-foreground">{t('wizard.done.enableFirst')}</p>}
      {mapping && <p className="text-xs text-muted-foreground">{t('wizard.done.previewHint')}</p>}
      {mapping && (
        <MappingPreviewPanel
          providerId={provider.id}
          providerType={provider.providerType}
          config={mapping}
          suggestedGroups={test.data?.groups}
          canFetchGroups={Boolean(spec?.Capabilities.SupportsGroupFetch)}
        />
      )}
      <IdentityProviderTestDialog
        result={test.data}
        pending={test.isPending}
        providerName={provider.name}
        open={testOpen}
        onOpenChange={setTestOpen}
      />
    </div>
  );
}
