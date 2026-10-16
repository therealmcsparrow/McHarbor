// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import type { VM, VMState } from '../types';

/**
 * A guest's state.
 *
 * `unknown` gets a tooltip rather than a colour alone, because it is the one
 * state that means something an operator will not guess: the host cannot be
 * reached, the guest is probably still running under systemd, and nothing can
 * be confirmed until the agent returns. A grey badge on its own would read as
 * "off", which is a different and wrong claim.
 */
export function VMStateBadge({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const label = t(`state.${vm.state}`);

  if (vm.state === 'deleting') {
    const percent = vm.deleteProgress?.percent;
    return <Badge variant="warning">{percent === undefined ? label : `${label} ${percent}%`}</Badge>;
  }

  if (vm.runState === 'io-error') {
    // QEMU paused this guest because a disk backend failed, almost always a
    // full pool. "Paused" on its own reads as something the operator did, so
    // the sub-state gets its own badge rather than a tooltip on the old one.
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="destructive">{t('state.ioError')}</Badge>
        </TooltipTrigger>
        <TooltipContent className="max-w-[320px]">{t('state.ioErrorHint')}</TooltipContent>
      </Tooltip>
    );
  }

  if (vm.runState === 'guest-panicked') {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="destructive">{t('state.guestPanicked')}</Badge>
        </TooltipTrigger>
        <TooltipContent className="max-w-[320px]">{t('state.guestPanickedHint')}</TooltipContent>
      </Tooltip>
    );
  }

  if (vm.state === 'unknown') {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="warning">{label}</Badge>
        </TooltipTrigger>
        <TooltipContent>{t('state.unknownHint')}</TooltipContent>
      </Tooltip>
    );
  }

  if (vm.state === 'crashed' && vm.lastError) {
    // The journal's own reason, which is what turns "it died" into "the OOM
    // killer took it".
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="destructive">{label}</Badge>
        </TooltipTrigger>
        <TooltipContent>{vm.lastError}</TooltipContent>
      </Tooltip>
    );
  }

  return <Badge variant={variantFor(vm.state)}>{label}</Badge>;
}

function variantFor(state: VMState): 'success' | 'destructive' | 'warning' | 'secondary' | 'outline' {
  switch (state) {
    case 'running':
      return 'success';
    case 'crashed':
      return 'destructive';
    case 'unknown':
    case 'stopping':
    case 'migrating':
    case 'deleting':
      return 'warning';
    case 'paused':
      return 'secondary';
    default:
      return 'outline';
  }
}
