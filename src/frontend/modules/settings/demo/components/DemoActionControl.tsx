// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type ComponentProps } from 'react';
import { IconDoorExit, IconFlask2, IconRestore, type TablerIcon } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { useDemoActionCopy } from '../hooks/useDemoActionCopy';
import type { DemoAction } from '@resources/types/demo';
import { DemoActionDialog } from './DemoActionDialog';

const ICONS: Record<DemoAction, TablerIcon> = {
  enter: IconFlask2,
  exit: IconDoorExit,
  reset: IconRestore,
};

type ButtonProps = ComponentProps<typeof Button>;

type DemoActionControlProps = {
  action: DemoAction;
  active: boolean;
  variant?: ButtonProps['variant'];
  size?: ButtonProps['size'];
  disabled?: boolean;
  compact?: boolean;
};

export function DemoActionControl({
  action,
  active,
  variant,
  size,
  disabled = false,
  compact = false,
}: DemoActionControlProps) {
  const [isOpen, setOpen] = useState(false);
  const copy = useDemoActionCopy(action, active);
  const Icon = ICONS[action];

  return (
    <>
      <Button
        variant={variant}
        size={size}
        permission={PERM.SYSTEM_DEMO}
        disabled={disabled}
        onClick={() => setOpen(true)}
      >
        <Icon className="size-4" aria-hidden />
        {compact ? copy.shortLabel : copy.label}
      </Button>
      <DemoActionDialog action={action} active={active} open={isOpen} onOpenChange={setOpen} />
    </>
  );
}
