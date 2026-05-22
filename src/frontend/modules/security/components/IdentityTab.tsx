// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconFingerprint } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useIdentityProviders, type IdentityProvider } from '../hooks/useIdentityProviders';
import { ProviderCard } from './ProviderCard';
import { CreateProviderDialog } from './CreateProviderDialog';
import { EditProviderDialog } from './EditProviderDialog';

export function IdentityTab() {
  const { t } = useTranslation('security');
  const { data: providers, isLoading } = useIdentityProviders();
  const [createOpen, setCreateOpen] = useState(false);
  const [editProvider, setEditProvider] = useState<IdentityProvider | null>(null);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold text-foreground">{t('identity.title')}</h3>
          <p className="text-sm text-muted-foreground">{t('identity.description')}</p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <IconPlus className="size-4" />
          {t('identity.addProvider')}
        </Button>
      </div>

      {providers && providers.length > 0 ? (
        <div className="space-y-3">
          {providers.map((provider) => (
            <ProviderCard
              key={provider.id}
              provider={provider}
              onEdit={setEditProvider}
            />
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border py-12">
          <IconFingerprint className="size-10 text-muted-foreground" />
          <div className="text-center">
            <p className="text-sm font-medium text-foreground">{t('identity.noProviders')}</p>
            <p className="mt-1 text-xs text-muted-foreground">{t('identity.noProvidersDescription')}</p>
          </div>
          <Button variant="outline" onClick={() => setCreateOpen(true)}>
            <IconPlus className="size-4" />
            {t('identity.addProvider')}
          </Button>
        </div>
      )}

      <CreateProviderDialog open={createOpen} onOpenChange={setCreateOpen} />

      {editProvider && (
        <EditProviderDialog
          open={!!editProvider}
          onOpenChange={(open) => { if (!open) setEditProvider(null); }}
          provider={editProvider}
        />
      )}
    </div>
  );
}
