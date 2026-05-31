// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentProps } from 'react';
import type { TFunction } from 'i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { NetworkInfo } from '@core/types/docker';
import type { MoveContainerPlan, MoveNetworkConfig } from '../hooks/useContainers';
import { MoveContainerOptions } from './MoveContainerOptions';
import { MoveContainerPlanSummary } from './MoveContainerPlanSummary';
import { MoveNetworkSettings } from './MoveNetworkSettings';

type TargetOption = {
  value: string;
  label: string;
};

type MoveContainerSetupProps = {
  t: TFunction<'containers'>;
  targetEnvId: string;
  targetName: string;
  targetOptions: TargetOption[];
  plan: MoveContainerPlan | undefined;
  planLoading: boolean;
  fallbackImage: string;
  networkMode: string;
  networks: MoveNetworkConfig[];
  targetNetworks: NetworkInfo[];
  moveOptions: ComponentProps<typeof MoveContainerOptions>['options'];
  onTargetEnvChange: (value: string) => void;
  onTargetNameChange: (value: string) => void;
  onNetworkModeChange: (value: string) => void;
  onNetworksChange: (value: MoveNetworkConfig[]) => void;
};

export function MoveContainerSetup({
  t,
  targetEnvId,
  targetName,
  targetOptions,
  plan,
  planLoading,
  fallbackImage,
  networkMode,
  networks,
  targetNetworks,
  moveOptions,
  onTargetEnvChange,
  onTargetNameChange,
  onNetworkModeChange,
  onNetworksChange,
}: MoveContainerSetupProps) {
  return (
    <>
      <div className="grid gap-3 md:grid-cols-2">
        <div className="space-y-1.5">
          <Label>{t('moveDialog.targetEnvironment')}</Label>
          <Select
            value={targetEnvId}
            onChange={onTargetEnvChange}
            options={targetOptions}
            placeholder={t('moveDialog.selectEnvironment')}
            ariaLabel={t('moveDialog.targetEnvironment')}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="move-target-name">{t('moveDialog.targetName')}</Label>
          <Input id="move-target-name" value={targetName} onChange={(event) => onTargetNameChange(event.target.value)} />
        </div>
      </div>

      {targetOptions.length === 0 ? (
        <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-200">
          {t('moveDialog.noTargets')}
        </div>
      ) : planLoading ? (
        <div className="flex h-36 items-center justify-center">
          <Spinner />
        </div>
      ) : plan ? (
        <>
          <MoveContainerPlanSummary plan={plan} fallbackImage={fallbackImage} />
          <MoveNetworkSettings
            networkMode={networkMode || plan.networkMode || 'bridge'}
            networks={networks}
            plan={plan}
            targetNetworks={targetNetworks}
            onNetworkModeChange={onNetworkModeChange}
            onNetworksChange={onNetworksChange}
          />
          <MoveContainerOptions options={moveOptions} />
        </>
      ) : null}
    </>
  );
}
