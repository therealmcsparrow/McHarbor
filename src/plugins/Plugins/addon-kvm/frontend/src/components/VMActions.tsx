// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconPlayerPause,
  IconPlayerPlay,
  IconPower,
  IconRotate,
  IconSkull,
  IconSparkles,
  IconTerminal2,
  IconTrash,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { GuestAgentDiscButton } from './GuestAgentDiscButton';
import { MakeTemplateButton } from './MakeTemplateButton';
import { VM_PERM } from '../permissions';
import type { LifecycleAction } from '../hooks/useVMs';
import type { VM } from '../types';

export type VMAction = LifecycleAction;

interface VMActionsProps {
  vm: VM;
  busy: boolean;
  diagnosePending?: boolean;
  onOpen: (id: string) => void;
  onAction: (action: VMAction) => void;
  onDelete: (vm: VM) => void;
  onDiagnose: (vm: VM) => void;
}

/**
 * The action buttons, shared by the table row and the card.
 *
 * The set and the enable rules are the detail header's, so an operator does
 * not have to open a guest to reach an action the header would have offered.
 * A guest whose host cannot be reached gets disabled buttons rather than
 * hidden ones — the operator should see that the control exists and why it
 * will not work.
 */
export function VMActions({
  vm,
  busy,
  diagnosePending = false,
  onOpen,
  onAction,
  onDelete,
  onDiagnose,
}: VMActionsProps) {
  const { t } = useTranslation('vm');

  const running = vm.state === 'running';
  const paused = vm.state === 'paused';
  const live = running || paused;
  // A guest on its way down is not startable yet: QEMU is still flushing and
  // the host would refuse the start until it has really gone.
  const stopping = vm.state === 'stopping';
  const disabled = busy || !vm.hostAvailable;
  const broken = vm.state === 'crashed' || Boolean(vm.lastError);

  return (
    <>
      {broken && (
        <IconAction
          label={t('actions.diagnose')}
          permission={VM_PERM.VIEW}
          disabled={diagnosePending}
          onClick={() => onDiagnose(vm)}
          icon={<IconSparkles className="h-4 w-4" aria-hidden="true" />}
        />
      )}
      {running && (
        <IconAction
          label={t('actions.console')}
          permission={VM_PERM.CONSOLE}
          disabled={disabled}
          onClick={() => onOpen(vm.id)}
          icon={<IconTerminal2 className="h-4 w-4" aria-hidden="true" />}
        />
      )}
      {live ? (
        <IconAction
          label={t('actions.shutdown')}
          permission={VM_PERM.MANAGE}
          disabled={disabled}
          onClick={() => onAction('shutdown')}
          icon={<IconPower className="h-4 w-4" aria-hidden="true" />}
        />
      ) : (
        <IconAction
          label={stopping ? t('actions.stillStopping') : t('actions.start')}
          permission={VM_PERM.MANAGE}
          // A template is deliberately not startable: that immutability is
          // what makes it a template.
          disabled={disabled || vm.isTemplate || stopping}
          onClick={() => onAction('start')}
          icon={<IconPlayerPlay className="h-4 w-4" aria-hidden="true" />}
        />
      )}
      {paused && (
        <IconAction
          label={t('actions.resume')}
          permission={VM_PERM.MANAGE}
          disabled={disabled}
          onClick={() => onAction('resume')}
          icon={<IconPlayerPlay className="h-4 w-4" aria-hidden="true" />}
        />
      )}
      <IconAction
        label={t('actions.pause')}
        permission={VM_PERM.MANAGE}
        disabled={disabled || !running}
        onClick={() => onAction('pause')}
        icon={<IconPlayerPause className="h-4 w-4" aria-hidden="true" />}
      />
      <IconAction
        label={t('actions.reset')}
        permission={VM_PERM.MANAGE}
        disabled={disabled || !live}
        onClick={() => onAction('reset')}
        icon={<IconRotate className="h-4 w-4" aria-hidden="true" />}
      />
      <GuestAgentDiscButton vm={vm} appearance="icon-sm" disabled={busy} />
      <MakeTemplateButton vm={vm} appearance="icon-sm" disabled={busy} />
      <span className="mx-0.5 h-5 w-px bg-border" aria-hidden="true" />
      <IconAction
        label={t('actions.stop')}
        permission={VM_PERM.MANAGE}
        variant="destructive"
        disabled={disabled || !live}
        onClick={() => onAction('stop')}
        icon={<IconSkull className="h-4 w-4" aria-hidden="true" />}
      />
      <IconAction
        label={t('actions.delete')}
        permission={VM_PERM.DELETE}
        variant="destructive"
        onClick={() => onDelete(vm)}
        icon={<IconTrash className="h-4 w-4" aria-hidden="true" />}
      />
    </>
  );
}

/**
 * One icon button in the row.
 *
 * Wrapped in a focusable span because a disabled button fires no pointer
 * events, and the tooltip is where a dead control explains itself.
 */
function IconAction({
  label,
  icon,
  onClick,
  permission,
  variant = 'ghost',
  disabled = false,
}: {
  label: string;
  icon: ReactNode;
  onClick: () => void;
  permission: string | string[];
  variant?: 'ghost' | 'destructive';
  disabled?: boolean;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span>
          <Button
            variant={variant}
            size="icon-sm"
            aria-label={label}
            permission={permission}
            disabled={disabled}
            onClick={onClick}
          >
            {icon}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * The guest's address, marked as the guest's own claim.
 *
 * Rendered as a text node and never as a link: this value comes from inside the
 * guest, and a guest is not something to take navigation instructions from.
 */
export function GuestAddress({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const address = vm.guestReportedIps?.[0];

  if (!address) {
    return <span className="text-muted-foreground">—</span>;
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="font-mono text-xs text-muted-foreground">{address}</span>
      </TooltipTrigger>
      <TooltipContent>{t('address.reportedHint')}</TooltipContent>
    </Tooltip>
  );
}
