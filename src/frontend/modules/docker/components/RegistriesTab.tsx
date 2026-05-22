// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconPlugConnected, IconPencil, IconTrash, IconCheck, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@resources/components/ui/Dialog';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  useRegistries,
  useCreateRegistry,
  useUpdateRegistry,
  useDeleteRegistry,
  useTestRegistry,
  type Registry,
  type CreateRegistryInput,
  type UpdateRegistryInput,
} from '../hooks/useRegistries';

export function RegistriesTab() {
  const { t } = useTranslation('docker');
  const { data, isLoading } = useRegistries();
  const [addOpen, setAddOpen] = useState(false);
  const [editReg, setEditReg] = useState<Registry | null>(null);
  const [deleteReg, setDeleteReg] = useState<Registry | null>(null);

  const registries = data?.items ?? [];

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('registries.title')}</h3>
          <p className="text-xs text-muted-foreground">{t('registries.description')}</p>
        </div>
        <Button size="sm" onClick={() => setAddOpen(true)}>
          <IconPlus className="size-4 mr-1.5" />
          {t('registries.add')}
        </Button>
      </div>

      {registries.length === 0 ? (
        <div className="py-12 text-center text-sm text-muted-foreground">
          {t('registries.noRegistries')}
        </div>
      ) : (
        <div className="rounded-lg border border-border overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('registries.columnName')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('registries.columnUrl')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('registries.columnUsername')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('registries.columnDefault')}</th>
                <th className="px-4 py-2.5 text-right font-medium text-muted-foreground">{t('registries.columnActions')}</th>
              </tr>
            </thead>
            <tbody>
              {registries.map((reg) => (
                <RegistryRow
                  key={reg.id}
                  registry={reg}
                  onEdit={() => setEditReg(reg)}
                  onDelete={() => setDeleteReg(reg)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <AddRegistryDialog open={addOpen} onOpenChange={setAddOpen} />
      {editReg && (
        <EditRegistryDialog
          open={!!editReg}
          onOpenChange={(open) => { if (!open) setEditReg(null); }}
          registry={editReg}
        />
      )}
      {deleteReg && (
        <DeleteRegistryDialog
          open={!!deleteReg}
          onOpenChange={(open) => { if (!open) setDeleteReg(null); }}
          registry={deleteReg}
        />
      )}
    </div>
  );
}

function RegistryRow({
  registry,
  onEdit,
  onDelete,
}: {
  registry: Registry;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation('docker');
  const testMutation = useTestRegistry();

  return (
    <tr className="border-b border-border last:border-0 hover:bg-muted/30">
      <td className="px-4 py-2.5 font-medium text-foreground">{registry.name}</td>
      <td className="px-4 py-2.5 text-muted-foreground">
        <code className="text-xs">{registry.url}</code>
      </td>
      <td className="px-4 py-2.5 text-muted-foreground">{registry.username || '-'}</td>
      <td className="px-4 py-2.5">
        {registry.isDefault && <Badge variant="default">{t('registries.columnDefault')}</Badge>}
      </td>
      <td className="px-4 py-2.5">
        <div className="flex items-center justify-end gap-1">
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('registries.testTooltip')}
            onClick={() => testMutation.mutate(registry.id)}
            disabled={testMutation.isPending}
          >
            {testMutation.isPending ? (
              <Spinner size="sm" />
            ) : testMutation.isSuccess ? (
              <IconCheck className="size-4 text-emerald-500" />
            ) : testMutation.isError ? (
              <IconX className="size-4 text-destructive" />
            ) : (
              <IconPlugConnected className="size-4" />
            )}
          </Button>
          <Button variant="ghost" size="icon" aria-label={t('registries.editTooltip')} onClick={onEdit}>
            <IconPencil className="size-4" />
          </Button>
          <Button variant="ghost" size="icon" aria-label={t('registries.removeTooltip')} onClick={onDelete}>
            <IconTrash className="size-4" />
          </Button>
        </div>
      </td>
    </tr>
  );
}

function AddRegistryDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
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
    <Dialog open={open} onOpenChange={onOpenChange}>
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

function EditRegistryDialog({
  open,
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
    updateMutation.mutate(
      { id: registry.id, input },
      { onSuccess: () => onOpenChange(false) },
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
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

function DeleteRegistryDialog({
  open,
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
    <Dialog open={open} onOpenChange={onOpenChange}>
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
        <label className="block text-sm font-medium text-foreground mb-1">{t('registries.nameLabel')}</label>
        <input
          type="text"
          value={form.name}
          onChange={(e) => onChange({ ...form, name: e.target.value })}
          className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground focus:border-primary focus:ring-1 focus:ring-primary"
          required
        />
      </div>
      <div>
        <label className="block text-sm font-medium text-foreground mb-1">{t('registries.urlLabel')}</label>
        <input
          type="text"
          value={form.url}
          onChange={(e) => onChange({ ...form, url: e.target.value })}
          placeholder="https://registry.example.com"
          className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground focus:border-primary focus:ring-1 focus:ring-primary"
          required
        />
      </div>
      <div>
        <label className="block text-sm font-medium text-foreground mb-1">{t('registries.usernameLabel')}</label>
        <input
          type="text"
          value={form.username}
          onChange={(e) => onChange({ ...form, username: e.target.value })}
          className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground focus:border-primary focus:ring-1 focus:ring-primary"
        />
      </div>
      <div>
        <label className="block text-sm font-medium text-foreground mb-1">
          {t('registries.passwordLabel')}
          {passwordOptional && <span className="text-muted-foreground ml-1">({t('common:labels.optional')})</span>}
        </label>
        <input
          type="password"
          value={form.password}
          onChange={(e) => onChange({ ...form, password: e.target.value })}
          className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground focus:border-primary focus:ring-1 focus:ring-primary"
          required={!passwordOptional}
        />
      </div>
    </>
  );
}
