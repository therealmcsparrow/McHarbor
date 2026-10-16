// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { PERM } from '@core/auth/permissionKeys';
import {
  useAPIKeyGrantOptions,
  useCreateAPIKey,
  useUpdateAPIKey,
  type APIKey,
  type CreateAPIKeyResult,
} from '../hooks/useAPIKeys';
import { useAPIKeyForm } from '../hooks/useAPIKeyForm';
import { APIKeyGrantFields } from './APIKeyGrantFields';

type APIKeyFormDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** null = create; a key = edit (name and grants only, never the secret). */
  existing: APIKey | null;
  onCreated: (result: CreateAPIKeyResult) => void;
};

export function APIKeyFormDialog({ open: isOpen, onOpenChange, existing, onCreated }: APIKeyFormDialogProps) {
  const { t } = useTranslation('security');
  const createKey = useCreateAPIKey();
  const updateKey = useUpdateAPIKey();
  const { data: options, isLoading: optionsLoading } = useAPIKeyGrantOptions(isOpen);
  const form = useAPIKeyForm(existing, isOpen);

  const expiryOptions = [
    { value: '', label: t('apiKeys.expirationOptions.never') },
    { value: '30', label: t('apiKeys.expirationOptions.30days') },
    { value: '90', label: t('apiKeys.expirationOptions.90days') },
    { value: '365', label: t('apiKeys.expirationOptions.1year') },
  ];

  const pending = createKey.isPending || updateKey.isPending;
  const canSubmit = form.state.name.trim().length > 0 && form.hasGrants && !pending;

  const handleSubmit = () => {
    if (existing) {
      updateKey.mutate(
        { id: existing.id, body: { name: form.state.name.trim(), ...form.grants } },
        { onSuccess: () => onOpenChange(false) },
      );
      return;
    }
    createKey.mutate(
      { name: form.state.name.trim(), expiresAt: form.resolveExpiresAt(), ...form.grants },
      { onSuccess: (data) => onCreated(data as CreateAPIKeyResult) },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{existing ? t('apiKeys.editKey') : t('apiKeys.createKey')}</DialogTitle>
        </DialogHeader>

        <div className="max-h-[75vh] space-y-4 overflow-y-auto py-2">
          <div className="space-y-2">
            <Label htmlFor="api-key-name">{t('apiKeys.name')}</Label>
            <Input
              id="api-key-name"
              placeholder={t('apiKeys.name')}
              value={form.state.name}
              onChange={(e) => form.setName(e.target.value)}
            />
          </div>

          {!existing && (
            <div className="space-y-2">
              <Label>{t('apiKeys.expiresAt')}</Label>
              <Select value={form.state.expiry} onChange={form.setExpiry} options={expiryOptions} />
            </div>
          )}

          <APIKeyGrantFields
            options={options}
            loading={optionsLoading}
            roleIds={form.state.roleIds}
            environmentIds={form.state.environmentIds}
            selection={form.state.selection}
            scopeCount={form.scopes.length}
            onRolesChange={form.setRoleIds}
            onEnvironmentsChange={form.setEnvironmentIds}
            onSelectionChange={form.setSelection}
          />

          {!form.hasGrants && <p className="text-sm text-warning">{t('apiKeys.grantsRequired')}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.API_KEYS_MANAGE} onClick={handleSubmit} disabled={!canSubmit}>
            {pending
              ? t('actions.processing', { ns: 'common' })
              : existing
                ? t('actions.save', { ns: 'common' })
                : t('actions.create', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
