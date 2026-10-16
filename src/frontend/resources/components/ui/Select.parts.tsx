// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Fragment, useEffect } from 'react';
import { cn } from '@resources/utils/cn';
import { PlainButton } from './PlainButton';
import type { SelectOption } from './Select';

// The menu is portalled, so when the select sits inside a Dialog it lands
// outside the subtree Radix's scroll lock treats as scrollable, and the lock
// cancels the wheel event before the list ever sees it — leaving a menu that
// only moved by dragging its scrollbar. Scrolling the viewport here and
// keeping the event to ourselves restores the wheel without unlocking the
// page behind the dialog.
export function useMenuWheelScroll(isOpen: boolean, viewportEl: HTMLDivElement | null) {
  useEffect(() => {
    if (!isOpen || !viewportEl) return;

    const handleWheel = (evt: WheelEvent) => {
      const limit = viewportEl.scrollHeight - viewportEl.clientHeight;
      if (limit <= 0) return;
      // deltaMode 1 is lines, 2 is pages; both need a pixel figure.
      const step = evt.deltaMode === 1 ? 16 : evt.deltaMode === 2 ? viewportEl.clientHeight : 1;
      const next = Math.max(0, Math.min(limit, viewportEl.scrollTop + evt.deltaY * step));
      if (next !== viewportEl.scrollTop) viewportEl.scrollTop = next;
      evt.preventDefault();
      evt.stopPropagation();
    };

    viewportEl.addEventListener('wheel', handleWheel, { passive: false });
    return () => viewportEl.removeEventListener('wheel', handleWheel);
  }, [isOpen, viewportEl]);
}

type SelectOptionRowProps = {
  option: SelectOption;
  showGroup: boolean;
  selected: boolean;
  onSelect: (value: string) => void;
};

export function SelectOptionRow({ option, showGroup, selected, onSelect }: SelectOptionRowProps) {
  const OptionIcon = option.icon;
  return (
    <Fragment>
      {showGroup && (
        <div
          role="presentation"
          className="px-3 pt-2 pb-1 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase"
        >
          {option.group}
        </div>
      )}
      <PlainButton
        onClick={() => onSelect(option.value)}
        className={cn(
          'flex w-full gap-x-2 rounded-md px-3 py-1.5 text-left text-sm transition-colors hover:bg-muted/50',
          option.description ? 'items-start' : 'items-center',
          selected ? 'bg-muted/70 font-medium text-foreground' : 'text-foreground',
        )}
      >
        {OptionIcon && (
          <OptionIcon
            className={cn(
              'size-4 shrink-0',
              option.description ? 'mt-0.5' : '',
              option.iconClassName ?? 'text-muted-foreground',
            )}
          />
        )}
        {option.description ? (
          <span className="flex min-w-0 flex-col">
            <span className="truncate">{option.label}</span>
            <span className="text-xs font-normal whitespace-normal text-muted-foreground">{option.description}</span>
          </span>
        ) : (
          <span className="truncate">{option.label}</span>
        )}
      </PlainButton>
    </Fragment>
  );
}
