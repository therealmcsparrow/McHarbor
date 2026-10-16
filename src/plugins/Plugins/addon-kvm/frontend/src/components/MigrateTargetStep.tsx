// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { networkLabel } from '../utils/nicAttachment';
import type { MigrateForm } from '../hooks/useMigrateForm';
import type { VM } from '../types';

export function hostName(form: MigrateForm, id: string, names: Map<string, string>): string {
  const host = form.targets.data?.find((entry) => entry.environmentId === id);
  return host?.name || names.get(id) || id;
}

/** Where the guest goes: host, pool, live or cold, and which networks its interfaces join. */
export function MigrateTargetStep({
  vm,
  form,
  names,
  liveAllowed,
  liveReason,
}: {
  vm: VM;
  form: MigrateForm;
  names: Map<string, string>;
  liveAllowed: boolean;
  liveReason: string;
}) {
  const { t } = useTranslation('vm');
  const hosts = form.targets.data ?? [];

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <Label>{t('migrate.target')}</Label>
        {hosts.length === 0 && !form.targets.isLoading ? (
          <p className="text-sm text-muted-foreground">{t('migrate.noTargets')}</p>
        ) : (
          <Select
            value={form.target}
            ariaLabel={t('migrate.target')}
            placeholder={t('migrate.targetPlaceholder')}
            options={hosts.map((host) => ({
              value: host.environmentId,
              label: hostName(form, host.environmentId, names),
            }))}
            onChange={form.chooseTarget}
          />
        )}
      </div>

      {form.target && (
        <div className="space-y-1">
          <Label>{t('migrate.pool')}</Label>
          <Select
            value={form.poolId}
            ariaLabel={t('migrate.pool')}
            placeholder={t('migrate.poolPlaceholder')}
            options={(form.pools.data ?? []).map((pool) => ({
              value: pool.id,
              label: `${pool.name} · ${pool.backend}`,
            }))}
            onChange={form.setPoolId}
          />
        </div>
      )}

      <div className="flex items-start justify-between gap-3">
        <div>
          <Label htmlFor="vm-migrate-live" className="text-sm font-normal">
            {t('migrate.live')}
          </Label>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {liveAllowed ? (form.live ? t('migrate.liveHint') : t('migrate.coldHint')) : liveReason}
          </p>
        </div>
        <Switch id="vm-migrate-live" checked={form.live} disabled={!liveAllowed} onCheckedChange={form.setLive} />
      </div>

      {form.live && (
        <div className="grid gap-3 rounded-md border border-border p-3 sm:grid-cols-3">
          <div className="space-y-1 sm:col-span-3">
            <Label htmlFor="vm-migrate-address">{t('migrate.address')}</Label>
            <Input
              id="vm-migrate-address"
              value={form.targetAddress}
              placeholder={t('migrate.addressPlaceholder')}
              onChange={(evt) => form.setTargetAddress(evt.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <div className="space-y-1 sm:col-span-3 md:col-span-1">
            <Label htmlFor="vm-migrate-bandwidth">{t('migrate.bandwidth')}</Label>
            <NumberInput
              id="vm-migrate-bandwidth"
              nullable
              value={form.maxBandwidthMb}
              min={1}
              onChange={form.setMaxBandwidthMb}
            />
          </div>
          <div className="space-y-1 sm:col-span-3 md:col-span-1">
            <Label htmlFor="vm-migrate-downtime">{t('migrate.downtime')}</Label>
            <NumberInput
              id="vm-migrate-downtime"
              nullable
              value={form.maxDowntimeMs}
              min={1}
              onChange={form.setMaxDowntimeMs}
            />
          </div>
        </div>
      )}

      {form.target && (vm.nics ?? []).length > 0 && (
        <div className="space-y-2">
          <Label>{t('migrate.networks')}</Label>
          <p className="text-xs text-muted-foreground">{t('migrate.networksHint')}</p>
          {(vm.nics ?? []).map((nic) => (
            <div key={nic.id} className="flex items-center gap-2">
              <span className="w-40 shrink-0 truncate font-mono text-xs text-muted-foreground">{nic.mac}</span>
              <Select
                value={form.nicNetworks[nic.id] ?? ''}
                ariaLabel={t('migrate.networkFor', { mac: nic.mac })}
                options={[
                  { value: '', label: t('migrate.keepBridge', { bridge: nic.bridge || nic.parentLink || '—' }) },
                  ...(form.networks.data ?? []).map((network) => ({
                    value: network.id,
                    label: networkLabel(network),
                  })),
                ]}
                onChange={(value) => form.setNicNetwork(nic.id, value)}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
