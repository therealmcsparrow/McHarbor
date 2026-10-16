// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFlask } from '@tabler/icons-react';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { PERM } from '@core/auth/permissionKeys';
import { usePermissionCatalog } from '@core/auth/usePermission';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import { useUsers } from '@resources/hooks/useUsers';
import { useSimulatePolicy, type SimulateInput } from '../../hooks/useGovernance';
import { SimulationResultView } from './SimulationResultView';

type Mode = 'permission' | 'route';

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'];

export function PolicySimulatorTab() {
  const { t } = useTranslation('security');
  const users = useUsers();
  const environments = useEnvironmentList();
  const catalog = usePermissionCatalog();
  const simulate = useSimulatePolicy();
  const [mode, setMode] = useState<Mode>('permission');
  const [input, setInput] = useState<SimulateInput>({
    userId: '',
    environmentId: '',
    stackName: '',
    permission: '',
    method: 'DELETE',
    path: '',
  });
  const set = (patch: Partial<SimulateInput>) => setInput((i) => ({ ...i, ...patch }));
  const ready = input.userId !== '' && (mode === 'permission' ? input.permission !== '' : input.path.trim() !== '');

  const run = () =>
    simulate.mutate(mode === 'permission' ? { ...input, method: '', path: '' } : { ...input, permission: '' });

  return (
    <SettingsGroup
      icon={IconFlask}
      title={t('governance.simulator.title')}
      description={t('governance.simulator.description')}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <Select
          ariaLabel={t('governance.simulator.user')}
          placeholder={t('governance.simulator.user')}
          value={input.userId}
          onChange={(userId) => set({ userId })}
          options={(users.data ?? []).map((u) => ({ value: u.id, label: u.username }))}
        />
        <Select
          ariaLabel={t('governance.simulator.environment')}
          value={input.environmentId}
          onChange={(environmentId) => set({ environmentId })}
          options={[
            { value: '', label: t('governance.simulator.global') },
            ...(environments.data ?? []).map((e) => ({ value: e.id, label: e.name })),
          ]}
        />
        <Input
          aria-label={t('governance.simulator.stack')}
          placeholder={t('governance.simulator.stack')}
          value={input.stackName}
          onChange={(e) => set({ stackName: e.target.value })}
        />
        <Select
          ariaLabel={t('governance.simulator.mode')}
          value={mode}
          onChange={(v) => setMode(v as Mode)}
          searchable={false}
          options={[
            { value: 'permission', label: t('governance.simulator.byPermission') },
            { value: 'route', label: t('governance.simulator.byRoute') },
          ]}
        />
        {mode === 'permission' ? (
          <div className="sm:col-span-2">
            <Select
              ariaLabel={t('governance.simulator.permission')}
              placeholder={t('governance.simulator.permission')}
              value={input.permission}
              onChange={(permission) => set({ permission })}
              options={(catalog.data ?? []).map((e) => ({ value: e.key, label: e.key, description: e.description }))}
            />
          </div>
        ) : (
          <div className="flex gap-2 sm:col-span-2">
            <div className="w-32">
              <Select
                ariaLabel={t('governance.simulator.method')}
                value={input.method}
                onChange={(method) => set({ method })}
                searchable={false}
                options={METHODS.map((m) => ({ value: m, label: m }))}
              />
            </div>
            <Input
              aria-label={t('governance.simulator.path')}
              placeholder={t('governance.simulator.pathPlaceholder')}
              value={input.path}
              onChange={(e) => set({ path: e.target.value })}
            />
          </div>
        )}
      </div>
      <div className="flex justify-end">
        <Button
          onClick={run}
          disabled={!ready || simulate.isPending}
          permission={[PERM.PERMISSIONS_VIEW, PERM.RBAC_POLICIES_VIEW]}
        >
          {t('governance.simulator.run')}
        </Button>
      </div>
      {simulate.isError && (
        <p role="alert" className="text-xs text-destructive">
          {simulate.error.message}
        </p>
      )}
      {simulate.data && <SimulationResultView result={simulate.data} />}
    </SettingsGroup>
  );
}
