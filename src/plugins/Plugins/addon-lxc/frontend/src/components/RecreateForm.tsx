// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { DialogFooter } from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { CT_PERM } from '../permissions';
import { useComputeNetworks } from '../hooks/useComputeNetworks';
import { useComputePools } from '../hooks/useComputePools';
import { useRecreateFromBackup, type RecreateDraft } from '../hooks/useRecreate';
import { useRecreateForm } from '../hooks/useRecreateForm';
import { RecreateInterfaces, RecreateLimits, RecreateMounts, RecreateWarnings } from './RecreateDefinitionFields';
import type { InitSystem } from '../types';

export function RecreateForm({
  draft,
  runId,
  environments,
  onCancel,
  onCreated,
}: {
  draft: RecreateDraft;
  runId: string;
  environments: { id: string; name: string }[];
  onCancel: () => void;
  onCreated: (containerId: string) => void;
}) {
  const { t } = useTranslation('ct');
  const form = useRecreateForm(draft);
  const recreate = useRecreateFromBackup();
  const manageGate = usePermissionGate(CT_PERM.MANAGE);
  const pools = useComputePools(form.environmentId || undefined);
  const networks = useComputeNetworks(form.environmentId || undefined);
  const pool = form.resolvePool(pools.data ?? []);
  const poolMissing = pools.isSuccess && !pool;

  const hosts = environments.some((env) => env.id === draft.create.environmentId)
    ? environments
    : [{ id: draft.create.environmentId, name: draft.create.environmentId }, ...environments];

  const submit = () => {
    if (!pool) return;
    recreate.mutate(
      { runId, body: form.buildBody(pool) },
      { onSuccess: (result) => result && onCreated(result.container.id) },
    );
  };

  return (
    <>
      <div className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
        <RecreateWarnings warnings={draft.warnings} />

        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <Label htmlFor="ct-recreate-name">{t('list.columns.name')}</Label>
            <Input
              id="ct-recreate-name"
              value={form.name}
              autoComplete="off"
              spellCheck={false}
              onChange={(ev) => form.setName(ev.target.value)}
            />
          </div>
          <div>
            <p className="mb-1 text-sm font-medium text-foreground">{t('list.columns.host')}</p>
            <Select
              value={form.environmentId}
              ariaLabel={t('list.columns.host')}
              options={hosts.map((env) => ({ value: env.id, label: env.name }))}
              onChange={form.setEnvironmentId}
            />
          </div>
        </div>

        <div>
          <p className="mb-1 text-sm font-medium text-foreground">{t('recreate.dialog.pool')}</p>
          <Select
            value={pool?.id ?? ''}
            ariaLabel={t('recreate.dialog.pool')}
            placeholder={t('recreate.dialog.poolPlaceholder')}
            options={(pools.data ?? []).map((entry) => ({
              value: entry.id,
              label: `${entry.name} · ${entry.backend} · ${entry.target}`,
            }))}
            onChange={form.setPoolChoice}
          />
          {poolMissing && (
            <p className="mt-1 text-xs text-warning">
              {t('recreate.dialog.poolMissing', {
                backend: draft.create.poolBackend || '—',
                target: draft.create.poolTarget || '—',
              })}
            </p>
          )}
        </div>

        <div className="grid gap-3 sm:grid-cols-3">
          <div>
            <p className="mb-1 text-sm font-medium text-foreground">{t('create.profile.init')}</p>
            <Select
              value={form.initSystem}
              ariaLabel={t('create.profile.init')}
              options={[
                // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: init system name
                { value: 'systemd', label: 'systemd' },
                // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: init system name
                { value: 'openrc', label: 'OpenRC' },
                { value: 'none', label: t('state.defined') },
              ]}
              onChange={(value) => form.setInitSystem(value as InitSystem)}
            />
          </div>
          <div className="flex items-center justify-between gap-2 sm:pt-6">
            <Label htmlFor="ct-recreate-privileged">{t('create.profile.privileged')}</Label>
            <Switch id="ct-recreate-privileged" checked={form.privileged} onCheckedChange={form.setPrivileged} />
          </div>
          <div className="flex items-center justify-between gap-2 sm:pt-6">
            <Label htmlFor="ct-recreate-nesting">{t('create.profile.nesting')}</Label>
            <Switch id="ct-recreate-nesting" checked={form.nesting} onCheckedChange={form.setNesting} />
          </div>
        </div>

        <RecreateLimits
          memoryMb={form.memoryMb}
          cpuMax={form.cpuMax}
          pidsMax={form.pidsMax}
          onMemoryMb={form.setMemoryMb}
          onCpuMax={form.setCpuMax}
          onPidsMax={form.setPidsMax}
        />

        <RecreateInterfaces
          nics={form.nics}
          networks={networks.data ?? []}
          networksLoaded={networks.isSuccess}
          onChange={form.setNics}
        />
        <RecreateMounts mounts={form.mounts} onChange={form.setMounts} />

        <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">{t('recreate.dialog.restoreNote')}</p>
        <div className="flex items-center gap-2">
          <Checkbox
            id="ct-recreate-start"
            checked={form.startAfter}
            onCheckedChange={(checked) => form.setStartAfter(checked === true)}
          />
          <Label htmlFor="ct-recreate-start">{t('recreate.dialog.startAfter')}</Label>
        </div>

        {recreate.isError && (
          <p
            role="alert"
            className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
          >
            {recreate.error.message}
          </p>
        )}
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={onCancel}>
          {t('actions.cancel')}
        </Button>
        <Button
          permission={manageGate.denied ? CT_PERM.MANAGE : CT_PERM.RESTORE}
          disabled={!form.name.trim() || !pool || recreate.isPending}
          onClick={submit}
        >
          {recreate.isPending && <Spinner className="mr-2 h-4 w-4" />}
          {t('recreate.dialog.submit')}
        </Button>
      </DialogFooter>
    </>
  );
}
