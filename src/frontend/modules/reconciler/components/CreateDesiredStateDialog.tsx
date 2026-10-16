// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import type { UseMutationResult } from '@tanstack/react-query';
import { PERM } from '@core/auth/permissionKeys';
import { useStacks } from '@resources/hooks/useStacks';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { CreateDesiredStateInput } from '../hooks/useReconciler';
import { usePluginKinds, type PluginKind } from '../hooks/usePluginKinds';
import { ContainerKindFields } from './ContainerKindFields';
import { PluginKindFields, type PluginSelection } from './PluginKindFields';

type CreateDesiredStateDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  createState: UseMutationResult<unknown, Error, CreateDesiredStateInput>;
};

const EMPTY: CreateDesiredStateInput = {
  name: '',
  kind: 'container',
  containerName: '',
  imageRef: '',
  stackName: '',
  desiredStatus: 'running',
  mode: 'observe',
};
const PLUGIN_PREFIX = 'plugin:';

export function isCreatable(form: CreateDesiredStateInput, plugin?: PluginSelection): boolean {
  if (!form.name.trim()) return false;
  if (plugin) return plugin.subjectId !== '' && Object.values(plugin.spec).some((v) => v.trim() !== '');
  if (form.kind === 'container') return form.containerName.trim() !== '' && form.imageRef.trim() !== '';
  if (form.kind === 'stack') return form.stackName !== '';
  return true;
}

export function CreateDesiredStateDialog({ open: isOpen, onOpenChange, createState }: CreateDesiredStateDialogProps) {
  const { t } = useTranslation('common');
  const currentEnv = useEnvironmentStore((s) => s.currentId) ?? '';
  const [form, setForm] = useState<CreateDesiredStateInput>(EMPTY);
  const [kindKey, setKindKey] = useState('container');
  const [selection, setSelection] = useState<PluginSelection>({ env: '', subjectId: '', spec: {} });
  const stacks = useStacks();
  const kinds = usePluginKinds();
  const set = (patch: Partial<CreateDesiredStateInput>) => setForm((current) => ({ ...current, ...patch }));

  const pluginKind: PluginKind | undefined = kindKey.startsWith(PLUGIN_PREFIX)
    ? (kinds.data ?? []).find((k) => `${PLUGIN_PREFIX}${k.source}:${k.kind}` === kindKey)
    : undefined;

  const chooseKind = (key: string) => {
    setKindKey(key);
    const pk = (kinds.data ?? []).find((k) => `${PLUGIN_PREFIX}${k.source}:${k.kind}` === key);
    setSelection({ env: pk && !pk.ownScopes ? currentEnv : '', subjectId: '', spec: {} });
    set({ kind: pk ? pk.kind : key });
  };

  const reset = () => {
    setForm(EMPTY);
    setKindKey('container');
    setSelection({ env: '', subjectId: '', spec: {} });
  };

  const handleCreate = () => {
    if (!isCreatable(form, pluginKind ? selection : undefined)) return;
    const payload: CreateDesiredStateInput = pluginKind
      ? {
          ...form,
          name: form.name.trim(),
          source: pluginKind.source,
          subjectId: selection.subjectId,
          spec: selection.spec,
          envId: selection.env,
        }
      : { ...form, name: form.name.trim(), containerName: form.containerName.trim(), imageRef: form.imageRef.trim() };
    createState.mutate(payload, {
      onSuccess: () => {
        onOpenChange(false);
        reset();
      },
    });
  };

  const kindOptions = [
    { value: 'container', label: t('kind.container', { ns: 'reconciler' }) },
    { value: 'stack', label: t('kind.stack', { ns: 'reconciler' }) },
    { value: 'environment', label: t('kind.environment', { ns: 'reconciler' }) },
    ...(kinds.data ?? []).map((k) => ({
      value: `${PLUGIN_PREFIX}${k.source}:${k.kind}`,
      label: k.label,
      description: k.source,
    })),
  ];

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('reconciler.addTitle')}</DialogTitle>
          <DialogDescription>{t('reconciler.addDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('reconciler.nameLabel')}</Label>
            <Input
              type="text"
              value={form.name}
              onChange={(e) => set({ name: e.target.value })}
              placeholder={t('placeholders.serviceName')}
            />
          </div>
          <div>
            <Label className="mb-2">{t('kind.label', { ns: 'reconciler' })}</Label>
            <Select
              value={kindKey}
              onChange={chooseKind}
              ariaLabel={t('kind.label', { ns: 'reconciler' })}
              options={kindOptions}
            />
            {!pluginKind && (
              <p className="mt-1 text-xs text-muted-foreground">{t(`kindHint.${form.kind}`, { ns: 'reconciler' })}</p>
            )}
          </div>
          {pluginKind && <PluginKindFields kind={pluginKind} value={selection} onChange={setSelection} />}
          {!pluginKind && form.kind === 'container' && <ContainerKindFields form={form} set={set} />}
          {!pluginKind && form.kind === 'stack' && (
            <div>
              <Label className="mb-2">{t('stackLabel', { ns: 'reconciler' })}</Label>
              <Select
                value={form.stackName}
                onChange={(value) => set({ stackName: value })}
                ariaLabel={t('stackLabel', { ns: 'reconciler' })}
                placeholder={t('stackPlaceholder', { ns: 'reconciler' })}
                searchable
                options={(stacks.data ?? []).map((stack) => ({ value: stack.name, label: stack.name }))}
              />
            </div>
          )}
          {!pluginKind && form.kind !== 'environment' && (
            <div>
              <Label className="mb-2">{t('reconciler.desiredStatusLabel')}</Label>
              <Select
                value={form.desiredStatus}
                onChange={(value) => set({ desiredStatus: value })}
                ariaLabel={t('reconciler.desiredStatusLabel')}
                options={[
                  { value: 'running', label: t('reconciler.statusRunning') },
                  { value: 'stopped', label: t('reconciler.statusStopped') },
                  { value: 'removed', label: t('reconciler.statusRemoved') },
                ]}
              />
            </div>
          )}
          <div>
            <Label className="mb-2">{t('mode.label', { ns: 'reconciler' })}</Label>
            <Select
              value={form.mode}
              onChange={(value) => set({ mode: value as CreateDesiredStateInput['mode'] })}
              ariaLabel={t('mode.label', { ns: 'reconciler' })}
              options={[
                { value: 'observe', label: t('mode.observe', { ns: 'reconciler' }) },
                { value: 'propose', label: t('mode.propose', { ns: 'reconciler' }) },
              ]}
            />
            <p className="mt-1 text-xs text-muted-foreground">{t(`mode.hint.${form.mode}`, { ns: 'reconciler' })}</p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button
            permission={PERM.RECONCILER_MANAGE}
            onClick={handleCreate}
            disabled={createState.isPending || !isCreatable(form, pluginKind ? selection : undefined)}
          >
            {createState.isPending ? t('reconciler.creating') : t('actions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
