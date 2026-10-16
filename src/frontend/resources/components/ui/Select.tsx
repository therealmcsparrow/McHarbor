// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useRef, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { cva, type VariantProps } from 'class-variance-authority';
import { IconChevronDown, IconSearch } from '@tabler/icons-react';
import * as Popover from '@radix-ui/react-popover';
import * as ScrollArea from '@radix-ui/react-scroll-area';
import { cn } from '@resources/utils/cn';
import { PlainButton } from './PlainButton';
import { SelectOptionRow, useMenuWheelScroll } from './Select.parts';

// Surface, focus and hover come from `.mc-field` / `.mc-field-btn` in app.css —
// the same material the text and number fields wear. Variants carry density only.
const selectTriggerVariants = cva(
  'mc-control mc-field mc-field-btn flex w-full items-center justify-between rounded-lg border text-left text-sm text-foreground transition-colors disabled:pointer-events-none disabled:opacity-50',
  {
    // Height is fixed by `.mc-control`; variants carry horizontal density only.
    variants: {
      variant: {
        default: 'px-4',
        outline: 'px-3',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

type SelectOption = {
  value: string;
  label: string;
  // Optional icon component rendered both in the trigger (when the
  // option is selected) and next to each row in the dropdown.
  // Used by the exec / terminal tab to show shell + user glyphs.
  icon?: React.ComponentType<{ className?: string }>;
  // Tailwind class applied to the icon (controls color).
  // Defaults to text-muted-foreground when omitted.
  iconClassName?: string;
  // Second line under the label in the dropdown row, for options whose
  // name does not explain what choosing them does. Not shown in the
  // trigger, which stays one line.
  description?: string;
  group?: string;
};

type SelectProps = VariantProps<typeof selectTriggerVariants> & {
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  searchable?: boolean;
  ariaLabel?: string;
  portalled?: boolean;
};

function Select({
  value,
  onChange,
  options,
  placeholder,
  disabled,
  className,
  variant,
  searchable = true,
  ariaLabel,
  portalled = true,
}: SelectProps) {
  const { t } = useTranslation('common');
  const [isOpen, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const searchRef = useRef<HTMLInputElement>(null);
  // A callback ref, not `useRef`: Radix's Portal renders null on its first
  // render and mounts its children from a layout effect, so a ref read during
  // the commit that opens the menu is still null. Observing the node as state
  // lets the wheel effect below run on the commit it actually appears.
  const [viewportEl, setViewportEl] = useState<HTMLDivElement | null>(null);

  const selectedOption = useMemo(() => options.find((o) => o.value === value), [options, value]);
  const selectedLabel = selectedOption?.label;
  const SelectedIcon = selectedOption?.icon;
  const selectedIconClass = selectedOption?.iconClassName ?? 'text-muted-foreground';

  const filtered = useMemo(() => {
    if (!search) return options;
    const q = search.toLowerCase();
    return options.filter((o) => o.label.toLowerCase().includes(q));
  }, [options, search]);

  if (!isOpen && search !== '') setSearch('');

  useEffect(() => {
    if (!isOpen || !searchable) return;
    const timer = window.setTimeout(() => searchRef.current?.focus(), 0);
    return () => window.clearTimeout(timer);
  }, [isOpen, searchable]);

  useMenuWheelScroll(isOpen, viewportEl);

  const handleSelect = (optionValue: string) => {
    onChange(optionValue);
    setOpen(false);
  };

  const resolvedPlaceholder = placeholder ?? t('select.placeholder');

  const content = (
    <Popover.Content
      sideOffset={4}
      align="start"
      className="mc-glass-overlay mc-menu z-50 min-w-[var(--radix-popover-trigger-width)] overflow-hidden rounded-lg"
    >
      {searchable && (
        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <IconSearch className="size-4 shrink-0 text-muted-foreground" />
          <input
            ref={searchRef}
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('select.searchPlaceholder')}
            className="w-full bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
          />
        </div>
      )}

      <ScrollArea.Root className="max-h-60 overflow-hidden">
        <ScrollArea.Viewport ref={setViewportEl} className="max-h-60 w-full p-1">
          {filtered.length === 0 ? (
            <div className="px-3 py-2 text-sm text-muted-foreground">{t('select.noResults')}</div>
          ) : (
            filtered.map((option, index) => (
              <SelectOptionRow
                key={option.value}
                option={option}
                showGroup={option.group !== undefined && option.group !== filtered[index - 1]?.group}
                selected={option.value === value}
                onSelect={handleSelect}
              />
            ))
          )}
        </ScrollArea.Viewport>
        <ScrollArea.Scrollbar orientation="vertical" className="flex w-2 touch-none p-0.5 select-none">
          <ScrollArea.Thumb className="relative flex-1 rounded-full bg-border" />
        </ScrollArea.Scrollbar>
      </ScrollArea.Root>
    </Popover.Content>
  );

  return (
    <Popover.Root open={isOpen} onOpenChange={setOpen}>
      <Popover.Trigger asChild disabled={disabled}>
        <PlainButton aria-label={ariaLabel} className={cn(selectTriggerVariants({ variant }), className)}>
          <span className="flex min-w-0 items-center gap-x-2">
            {SelectedIcon && <SelectedIcon className={cn('size-4 shrink-0', selectedIconClass)} />}
            <span className={cn('truncate', !selectedLabel && 'text-muted-foreground')}>
              {selectedLabel ?? resolvedPlaceholder}
            </span>
          </span>
          <IconChevronDown
            className={cn('ml-2 size-4 shrink-0 text-muted-foreground transition-transform', isOpen && 'rotate-180')}
          />
        </PlainButton>
      </Popover.Trigger>

      {portalled ? <Popover.Portal>{content}</Popover.Portal> : content}
    </Popover.Root>
  );
}

Select.displayName = 'Select';

export { Select, selectTriggerVariants };
export type { SelectOption, SelectProps };
