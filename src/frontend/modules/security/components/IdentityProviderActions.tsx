// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// IdentityProviderActions — the per-row controls for one identity provider.
//
// This is the right-hand half of what used to be `ProviderCard`: the enable
// switch, test, edit and delete, plus the two dialogs each row owns. The
// left-hand half (icon, name, type, status) became table columns when
// IdentityTab moved onto `TablePage`.
//
// The dialogs stay *per row* rather than being lifted into the tab. A test
// run is scoped to one provider and `useTestProvider` holds that provider's
// result; hoisting the state would mean re-plumbing which row a result
// belongs to for no gain.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash, IconPencil, IconPlugConnected } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  useUpdateProvider,
  useDeleteProvider,
  useTestProvider,
  type IdentityProvider as IdentityProviderValue,
} from '../hooks/useIdentityProviders';
import { IdentityProviderTestDialog } from './IdentityProviderTestDialog';
import { PERM } from '@core/auth/permissionKeys';

type IdentityProviderActionsProps = {
  provider: IdentityProviderValue;
  onEdit: (provider: IdentityProviderValue) => void;
};

export function IdentityProviderActions({ provider, onEdit }: IdentityProviderActionsProps) {
  const { t } = useTranslation('security');
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [testOpen, setTestOpen] = useState(false);
  const updateProvider = useUpdateProvider();
  const deleteProvider = useDeleteProvider();
  const testProvider = useTestProvider();

  // The modal opens first and shows a spinner, so a slow upstream
  // reads as progress rather than a dead button.
  function handleTest() {
    setTestOpen(true);
    testProvider.mutate(provider.id);
  }

  function handleDelete() {
    deleteProvider.mutate(provider.id, {
      onSuccess: () => setDeleteOpen(false),
    });
  }

  return (
    <>
      <div className="flex items-center justify-end gap-2">
        <Switch
          checked={provider.enabled}
          onCheckedChange={(enabled) => updateProvider.mutate({ id: provider.id, enabled })}
          disabled={updateProvider.isPending}
          aria-label={provider.enabled ? t('identity.enabled') : t('identity.disabled')}
        />
        <Button
          permission={PERM.IDENTITY_PROVIDERS_MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('identity.testConnection')}
          onClick={handleTest}
          disabled={testProvider.isPending}
        >
          <IconPlugConnected className="size-4" />
        </Button>
        <Button
          permission={PERM.IDENTITY_PROVIDERS_MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('identity.editProvider')}
          onClick={() => onEdit(provider)}
        >
          <IconPencil className="size-4" />
        </Button>
        <Button
          permission={PERM.IDENTITY_PROVIDERS_MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('identity.deleteProvider')}
          onClick={() => setDeleteOpen(true)}
        >
          <IconTrash className="size-4 text-destructive" />
        </Button>
      </div>

      <IdentityProviderTestDialog
        open={testOpen}
        onOpenChange={setTestOpen}
        result={testProvider.data}
        pending={testProvider.isPending}
        providerName={provider.name}
      />

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
