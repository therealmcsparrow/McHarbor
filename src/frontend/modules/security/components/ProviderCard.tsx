// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBrandAzure, IconBrandGoogle, IconTrash, IconPencil, IconPlugConnected } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Switch } from '@resources/components/ui/Switch';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { useUpdateProvider, useDeleteProvider, useTestProvider, type IdentityProvider } from '../hooks/useIdentityProviders';

type ProviderCardProps = {
  provider: IdentityProvider;
  onEdit: (provider: IdentityProvider) => void;
};

export function ProviderCard({ provider, onEdit }: ProviderCardProps) {
  const { t } = useTranslation('security');
  const [deleteOpen, setDeleteOpen] = useState(false);
  const updateProvider = useUpdateProvider();
  const deleteProvider = useDeleteProvider();
  const testProvider = useTestProvider();

  const Icon = provider.providerType === 'entra_id' ? IconBrandAzure : IconBrandGoogle;
  const iconColor = provider.providerType === 'entra_id' ? 'text-[#0078D4]' : 'text-[#4285F4]';
  const typeLabel = provider.providerType === 'entra_id'
    ? t('identity.entraId')
    : t('identity.google');

  function handleToggle(enabled: boolean) {
    updateProvider.mutate({ id: provider.id, enabled });
  }

  function handleDelete() {
    deleteProvider.mutate(provider.id, {
      onSuccess: () => setDeleteOpen(false),
    });
  }

  return (
    <>
      <div className="flex items-center gap-4 rounded-lg border border-border bg-card p-4">
        <Icon className={`size-8 shrink-0 ${iconColor}`} />

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h4 className="truncate text-sm font-medium text-foreground">
              {provider.name}
            </h4>
            <Badge variant={provider.enabled ? 'default' : 'secondary'}>
              {provider.enabled ? t('identity.enabled') : t('identity.disabled')}
            </Badge>
          </div>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {typeLabel}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Switch
            checked={provider.enabled}
            onCheckedChange={handleToggle}
            disabled={updateProvider.isPending}
          />
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('identity.testConnection')}
            onClick={() => testProvider.mutate(provider.id)}
            disabled={testProvider.isPending}
          >
            <IconPlugConnected className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('identity.editProvider')}
            onClick={() => onEdit(provider)}
          >
            <IconPencil className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('identity.deleteProvider')}
            onClick={() => setDeleteOpen(true)}
          >
            <IconTrash className="size-4 text-destructive" />
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('identity.deleteProvider')}
        description={t('identity.deleteProviderConfirm')}
        onConfirm={handleDelete}
        loading={deleteProvider.isPending}
        variant="destructive"
      />
    </>
  );
}
