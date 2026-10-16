// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { NetworkMode } from '../types';

interface NetworkBasicsFieldsProps {
  environments: { id: string; name: string }[];
  environmentId: string;
  onEnvironmentChange: (value: string) => void;
  editing: boolean;
  networkName: string;
  onNameChange: (value: string) => void;
  mode: NetworkMode;
  onModeChange: (value: NetworkMode) => void;
}

export function NetworkBasicsFields({
  environments,
  environmentId,
  onEnvironmentChange,
  editing,
  networkName,
  onNameChange,
  mode,
  onModeChange,
}: NetworkBasicsFieldsProps) {
  const { t } = useTranslation('compute');

  return (
    <>
      <div>
        <Label>{t('networks.create.environment')}</Label>
        <Select
          value={environmentId}
          disabled={editing}
          ariaLabel={t('networks.create.environment')}
          options={environments.map((env) => ({ value: env.id, label: env.name }))}
          onChange={onEnvironmentChange}
        />
      </div>

      <div>
        <Label htmlFor="net-name">{t('networks.create.name')}</Label>
        <Input id="net-name" value={networkName} onChange={(ev) => onNameChange(ev.target.value)} autoComplete="off" />
      </div>

      <div>
        <Label>{t('networks.create.mode')}</Label>
        <Select
          value={mode}
          disabled={editing}
          ariaLabel={t('networks.create.mode')}
          options={[
            { value: 'bridged', label: t('networks.mode.bridged') },
            { value: 'managed', label: t('networks.mode.managed') },
            { value: 'external', label: t('networks.mode.external') },
            { value: 'macvlan', label: t('networks.mode.macvlan') },
          ]}
          onChange={(value) => onModeChange(value as NetworkMode)}
        />
        <p className="mt-1 text-xs text-muted-foreground">
          {editing ? t('networks.edit.modeLocked') : t(`networks.mode.${mode}Hint`)}
        </p>
      </div>
    </>
  );
}
