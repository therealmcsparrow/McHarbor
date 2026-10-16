// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBaselineDensityMedium } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Popover, PopoverTrigger, PopoverContent } from '@resources/components/ui/Popover';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { cn } from '@resources/utils/cn';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';
import { TABLE_DENSITIES, type TableDensity } from './types';

type TableDensityMenuProps = {
  value: TableDensity;
  onChange: (next: TableDensity) => void;
  disabled?: boolean;
};

const densityLabelKeys: Record<TableDensity, string> = {
  compact: 'table.densityCompact',
  default: 'table.densityDefault',
  comfortable: 'table.densityComfortable',
};

export function TableDensityMenu({ value, onChange, disabled }: TableDensityMenuProps) {
  const { t } = useTranslation('common');

  const trigger = (
    <Button variant="outline" size="icon-sm" aria-label={t('table.densityLabel')} disabled={disabled}>
      <IconBaselineDensityMedium className="size-4" />
    </Button>
  );

  if (disabled) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <TooltipFocusTarget>{trigger}</TooltipFocusTarget>
        </TooltipTrigger>
        <TooltipContent>{t('tableViews.unavailable')}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Popover>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent className="w-44 p-2">
        <span className="mb-2 block text-xs font-medium text-muted-foreground">{t('table.density')}</span>
        <div className="flex flex-col gap-1">
          {TABLE_DENSITIES.map((density) => (
            <Button
              key={density}
              variant="ghost"
              size="sm"
              className={cn('justify-start', density === value && 'bg-muted text-foreground')}
              onClick={() => onChange(density)}
            >
              {t(densityLabelKeys[density])}
            </Button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
