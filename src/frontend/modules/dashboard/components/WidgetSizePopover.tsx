// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The exact-size control behind the widget header's resize button. Both
// resize paths stay available: drag the corner, or set exact numbers here
// for keyboard and precision use. Split out of WidgetShell.tsx unchanged.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowsMaximize, IconMinus, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Popover, PopoverContent, PopoverTrigger } from '@resources/components/ui/Popover';
import { PERM } from '@core/auth/permissionKeys';

type WidgetSizePopoverProps = {
  title: string;
  width: number;
  height: number;
  onResize: (w: number, h: number) => void;
};

export function WidgetSizePopover({ title, width, height, onResize }: WidgetSizePopoverProps) {
  const { t } = useTranslation('dashboard');
  const [sizeOpen, setSizeOpen] = useState(false);

  return (
    <Popover open={sizeOpen} onOpenChange={setSizeOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t('widget.size')}
          className="size-6 text-muted-foreground/50 hover:text-foreground"
        >
          <IconArrowsMaximize className="size-3.5" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-60 p-3">
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <label
              htmlFor={`widget-width-${title}`}
              className="flex items-center justify-between text-xs font-medium text-foreground"
            >
              {t('widget.width')}
              <span className="text-primary tabular-nums">{width}</span>
            </label>
            <input
              id={`widget-width-${title}`}
              type="range"
              value={width}
              min={1}
              max={12}
              step={1}
              onChange={(e) => onResize(parseInt(e.target.value, 10), height)}
              className="h-1 w-full cursor-pointer appearance-none rounded-full bg-muted accent-primary [&::-moz-range-thumb]:size-3 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:bg-primary [&::-webkit-slider-thumb]:size-3 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-primary"
            />
          </div>

          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-foreground">{t('widget.height')}</span>
            <div className="flex items-center gap-1">
              <Button
                permission={PERM.WIDGETS_MANAGE}
                type="button"
                variant="outline"
                size="icon-sm"
                aria-label={t('widget.decreaseHeight')}
                onClick={() => onResize(width, Math.max(1, height - 1))}
                className="size-6"
              >
                <IconMinus className="size-3" />
              </Button>
              <span className="min-w-6 text-center text-xs font-semibold text-foreground tabular-nums">{height}</span>
              <Button
                permission={PERM.WIDGETS_MANAGE}
                type="button"
                variant="outline"
                size="icon-sm"
                aria-label={t('widget.increaseHeight')}
                onClick={() => onResize(width, height + 1)}
                className="size-6"
              >
                <IconPlus className="size-3" />
              </Button>
            </div>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
