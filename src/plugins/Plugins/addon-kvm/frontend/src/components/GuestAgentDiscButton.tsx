// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDisc } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { VM_PERM } from '../permissions';
import { useAttachGuestAgentDisc } from '../hooks/useVMs';
import type { VM } from '../types';

interface GuestAgentDiscButtonProps {
  vm: VM;
  appearance?: 'header' | 'icon-sm' | 'labelled';
  disabled?: boolean;
  /** The host can put a disc into this running machine's empty drive. */
  live?: boolean;
}

export function GuestAgentDiscButton({
  vm,
  appearance = 'icon-sm',
  disabled = false,
  live = false,
}: GuestAgentDiscButtonProps) {
  const { t } = useTranslation('vm');
  const attach = useAttachGuestAgentDisc();

  const running = !live && (vm.state === 'running' || vm.state === 'paused');
  const blocked = disabled || running || !vm.hostAvailable || attach.isPending;
  const label = t('guestAgent.disc.attach');
  const hint = running ? t('hardware.mustBeStopped') : t('guestAgent.disc.hint');
  const insert = () => attach.mutate({ id: vm.id });

  const icon = attach.isPending ? (
    <Spinner className="size-3.5" />
  ) : (
    <IconDisc className="size-3.5" aria-hidden="true" />
  );

  if (appearance === 'header') {
    return (
      <HeaderActionButton
        tooltip={`${label} — ${hint}`}
        permission={VM_PERM.MANAGE}
        onClick={insert}
        disabled={blocked}
        icon={icon}
      />
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span>
          {appearance === 'labelled' ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="gap-1.5"
              permission={VM_PERM.MANAGE}
              disabled={blocked}
              onClick={insert}
            >
              {icon}
              {label}
            </Button>
          ) : (
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={label}
              permission={VM_PERM.MANAGE}
              disabled={blocked}
              onClick={insert}
            >
              {icon}
            </Button>
          )}
        </span>
      </TooltipTrigger>
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  );
}
