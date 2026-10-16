// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@resources/components/ui/Dialog';
import {
  useCreateRegistry,
  useUpdateRegistry,
  useDeleteRegistry,
  type Registry,
  type CreateRegistryInput,
  type UpdateRegistryInput,
} from '../hooks/useRegistries';

export function AddRegistryDialog({
  open: isOpen,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('docker');
  const createMutation = useCreateRegistry();
  const [form, setForm] = useState<CreateRegistryInput>({ name: '', url: '', username: '', password: '' });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    createMutation.mutate(form, {
      onSuccess: () => {
        onOpenChange(false);
        setForm({ name: '', url: '', username: '', password: '' });
      },
    });
  }

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogTitle>{t('registries.addTitle')}</DialogTitle>
        <DialogDescription>{t('registries.addDescription')}</DialogDescription>
        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          <RegistryFormFields form={form} onChange={setForm} />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              {t('common:actions.cancel')}
            </Button>
            <Button type="submit" disabled={createMutation.isPending || !form.name || !form.url}>
              {createMutation.isPending ? t('registries.adding') : t('common:actions.create')}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function EditRegistryDialog({
  open: isOpen,
  onOpenChange,
  registry,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  registry: Registry;
}) {
  const { t } = useTranslation('docker');
  const updateMutation = useUpdateRegistry();
  const [form, setForm] = useState<CreateRegistryInput>({
    name: registry.name,
    url: registry.url,
    username: registry.username,
    password: '',
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const input: UpdateRegistryInput = {
      name: form.name,
      url: form.url,
      username: form.username,
    };
    if (form.password) {
      input.password = form.password;
    }
    updateMutation.mutate({ id: registry.id, input }, { onSuccess: () => onOpenChange(false) });
  }

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogTitle>{t('registries.editTitle')}</DialogTitle>
        <DialogDescription>{t('registries.editDescription')}</DialogDescription>
        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          <RegistryFormFields form={form} onChange={setForm} passwordOptional />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              {t('common:actions.cancel')}
            </Button>
            <Button type="submit" disabled={updateMutation.isPending || !form.name || !form.url}>
              {updateMutation.isPending ? t('registries.saving') : t('common:actions.save')}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function DeleteRegistryDialog({
  open: isOpen,
  onOpenChange,
  registry,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  registry: Registry;
}) {
  const { t } = useTranslation('docker');
  const deleteMutation = useDeleteRegistry();

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogTitle>{t('registries.removeTitle')}</DialogTitle>
        <DialogDescription>{t('registries.removeDescription')}</DialogDescription>
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('common:actions.cancel')}
          </Button>
          <Button
            variant="destructive"
            disabled={deleteMutation.isPending}
            onClick={() =>
              deleteMutation.mutate(registry.id, {
                onSuccess: () => onOpenChange(false),
              })
            }
          >
            {t('common:actions.remove')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function RegistryFormFields({
  form,
  onChange,
  passwordOptional = false,
}: {
  form: CreateRegistryInput;
  onChange: (form: CreateRegistryInput) => void;
  passwordOptional?: boolean;
}) {
  const { t } = useTranslation('docker');

  return (
    <>
      <div>
        <label className="mb-1 block text-sm font-medium text-foreground">{t('registries.nameLabel')}</label>
        <input
          type="text"
          value={form.name}
          onChange={(e) => onChange({ ...form, name: e.target.value })}
          className="mc-control w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:ring-1 focus:ring-primary"
          required
        />
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium text-foreground">{t('registries.urlLabel')}</label>
        <input
          type="text"
          value={form.url}
          onChange={(e) => onChange({ ...form, url: e.target.value })}
          placeholder={t('registries.urlPlaceholder')}
          className="mc-control w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:ring-1 focus:ring-primary"
          required
        />
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium text-foreground">{t('registries.usernameLabel')}</label>
        <input
          type="text"
          value={form.username}
          onChange={(e) => onChange({ ...form, username: e.target.value })}
          className="mc-control w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:ring-1 focus:ring-primary"
        />
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium text-foreground">
          {t('registries.passwordLabel')}
          {passwordOptional && <span className="ml-1 text-muted-foreground">({t('common:labels.optional')})</span>}
        </label>
        <input
          type="password"
          value={form.password}
          onChange={(e) => onChange({ ...form, password: e.target.value })}
          className="mc-control w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:ring-1 focus:ring-primary"
          required={!passwordOptional}
        />
      </div>
    </>
  );
}
