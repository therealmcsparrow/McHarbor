// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type ComponentProps } from 'react';
import { IconAdjustmentsHorizontal, IconFlask2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { useDemoSizesCopy } from '../hooks/useDemoActionCopy';
import type { DemoSizes, DemoSizesMode } from '@resources/types/demo';
import { DemoSizesDialog } from './DemoSizesDialog';

type ButtonProps = ComponentProps<typeof Button>;

type DemoSizesControlProps = {
  mode: DemoSizesMode;
  sizes: DemoSizes;
  variant?: ButtonProps['variant'];
  disabled?: boolean;
};

export function DemoSizesControl({ mode, sizes, variant, disabled = false }: DemoSizesControlProps) {
  const [isOpen, setOpen] = useState(false);
  const [generation, setGeneration] = useState(0);
  const copy = useDemoSizesCopy(mode);
  const Icon = mode === 'enter' ? IconFlask2 : IconAdjustmentsHorizontal;

  return (
    <>
      <Button
        variant={variant}
        permission={PERM.SYSTEM_DEMO}
        disabled={disabled}
        onClick={() => {
          setGeneration((current) => current + 1);
          setOpen(true);
        }}
      >
        <Icon className="size-4" aria-hidden />
        {copy.label}
      </Button>
      <DemoSizesDialog key={generation} mode={mode} sizes={sizes} open={isOpen} onOpenChange={setOpen} />
    </>
  );
}
