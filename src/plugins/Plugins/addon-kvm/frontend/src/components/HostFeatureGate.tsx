// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import {
  machineTypeMissing,
  supportsFeature,
  useQEMUCapability,
  type QEMUCapability,
  type VMFeature,
} from '../hooks/useQEMUCapability';

export interface FeatureState {
  supported: boolean;
  known: boolean;
  label: string;
  reason: string;
  capability?: QEMUCapability;
}

export function useHostFeature(environmentId: string | undefined, feature: VMFeature): FeatureState {
  const { t } = useTranslation('vm');
  const { data: capability, isLoading, isError } = useQEMUCapability(environmentId);

  const label = t(`hostFeature.names.${feature}`);
  const known = Boolean(capability) && !isLoading && !isError;
  const supported = supportsFeature(capability, feature);

  return {
    supported,
    known,
    label,
    reason: known ? t('hostFeature.unavailable', { feature: label }) : t('hostFeature.unknown'),
    capability,
  };
}

interface HostFeatureGateProps {
  environmentId?: string;
  feature: VMFeature;
  children: (state: FeatureState) => ReactNode;
}

/**
 * Renders its child with the host's answer for one feature.
 *
 * The child is always rendered — a control the host cannot drive is shown
 * disabled with the reason, never hidden, so an operator can see the setting
 * exists and learn what their host is missing rather than wondering whether
 * McHarbor supports it at all.
 */
export function HostFeatureGate({ environmentId, feature, children }: HostFeatureGateProps) {
  const state = useHostFeature(environmentId, feature);
  return <>{children(state)}</>;
}

interface MachineTypeWarningProps {
  environmentId?: string;
  machineType?: string;
}

export function MachineTypeWarning({ environmentId, machineType }: MachineTypeWarningProps) {
  const { t } = useTranslation('vm');
  const { data: capability } = useQEMUCapability(environmentId);

  if (!machineTypeMissing(capability, machineType)) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex items-center gap-1 text-destructive">
          <IconAlertTriangle size={16} aria-hidden="true" />
          <span className="sr-only">{t('hostFeature.machineTypeMissing', { type: machineType })}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-[320px]">
        {t('hostFeature.machineTypeMissing', { type: machineType })}
      </TooltipContent>
    </Tooltip>
  );
}
