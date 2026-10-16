// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import type { StatusTone } from '@resources/theme/status-palette';
import type { VM, VMState } from '../types';

const TONES: Record<VMState, StatusTone> = {
  running: 'success',
  crashed: 'danger',
  stopping: 'warning',
  migrating: 'warning',
  deleting: 'warning',
  unknown: 'warning',
  paused: 'info',
  defined: 'neutral',
  stopped: 'neutral',
};

/**
 * The guest's state as the host's own status dot.
 *
 * `unknown` and a crash keep the tooltip the old badge carried: the first
 * means the host cannot be reached rather than "off", and the second carries
 * the journal's reason, which is what turns "it died" into "the OOM killer
 * took it".
 */
export function StateIndicator({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const label = t(`state.${vm.state}`);
  const indicator = (
    <StatusIndicator tone={TONES[vm.state] ?? 'neutral'} label={label} size="sm" pulse={vm.state === 'running'} />
  );

  const hint =
    vm.state === 'unknown' ? t('state.unknownHint') : vm.state === 'crashed' && vm.lastError ? vm.lastError : '';
  if (!hint) return indicator;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span>{indicator}</span>
      </TooltipTrigger>
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Whether the guest agent is answering.
 *
 * Worth its own badge because its absence changes two things an operator cares
 * about: snapshots become crash-consistent, and reported addresses stop
 * updating.
 */
export function GuestAgentBadge({ state }: { state?: string }) {
  const { t } = useTranslation('vm');
  if (!state || state === 'unknown') return null;

  if (state === 'connected') {
    return (
      <Badge variant="success" className="px-1.5 py-0 text-[10px]">
        {t('guestAgent.connected')}
      </Badge>
    );
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
          {t('guestAgent.absent')}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>{t('guestAgent.absentHint')}</TooltipContent>
    </Tooltip>
  );
}
