// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useRef, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { cva, type VariantProps } from 'class-variance-authority';
import { IconChevronDown, IconSearch } from '@tabler/icons-react';
import * as Popover from '@radix-ui/react-popover';
import * as ScrollArea from '@radix-ui/react-scroll-area';
import { cn } from '@resources/utils/cn';

const selectTriggerVariants = cva(
  'flex w-full items-center justify-between rounded-lg border text-sm text-foreground disabled:opacity-50 disabled:pointer-events-none',
  {
    variants: {
      variant: {
        default: 'py-2.5 px-4 bg-card border-border',
        outline: 'py-2 px-3 bg-background border-input focus:outline-none focus:ring-2 focus:ring-ring',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
);

type SelectOption = {
  value: string;
  label: string;
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
}: SelectProps) {
  const { t } = useTranslation('common');
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const searchRef = useRef<HTMLInputElement>(null);

  const selectedLabel = useMemo(
    () => options.find((o) => o.value === value)?.label,
    [options, value]
  );

  const filtered = useMemo(() => {
    if (!search) return options;
    const q = search.toLowerCase();
    return options.filter((o) => o.label.toLowerCase().includes(q));
  }, [options, search]);

  useEffect(() => {
    if (open && searchable) {
      setTimeout(() => searchRef.current?.focus(), 0);
    }
    if (!open) setSearch('');
  }, [open, searchable]);

  const handleSelect = (optionValue: string) => {
    onChange(optionValue);
    setOpen(false);
  };

  const resolvedPlaceholder = placeholder ?? t('select.placeholder');

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild disabled={disabled}>
        <button
          type="button"
          aria-label={ariaLabel}
          className={cn(selectTriggerVariants({ variant }), className)}
        >
          <span className={cn('truncate', !selectedLabel && 'text-muted-foreground')}>
            {selectedLabel ?? resolvedPlaceholder}
          </span>
          <IconChevronDown className={cn('ml-2 size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
        </button>
      </Popover.Trigger>

      <Popover.Portal>
        <Popover.Content
          sideOffset={4}
          align="start"
          className="z-50 min-w-[var(--radix-popover-trigger-width)] rounded-lg border border-border bg-card shadow-lg animate-in fade-in-0 zoom-in-95"
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
            <ScrollArea.Viewport className="max-h-60 w-full p-1">
              {filtered.length === 0 ? (
                <div className="px-3 py-2 text-sm text-muted-foreground">{t('select.noResults')}</div>
              ) : (
                filtered.map((option) => (
                  <button
                    key={option.value}
                    type="button"
                    onClick={() => handleSelect(option.value)}
                    className={cn(
                      'flex w-full items-center rounded-md px-3 py-1.5 text-left text-sm transition-colors hover:bg-muted/50',
                      option.value === value ? 'bg-muted/70 text-foreground font-medium' : 'text-foreground'
                    )}
                  >
                    {option.label}
                  </button>
                ))
              )}
            </ScrollArea.Viewport>
            <ScrollArea.Scrollbar
              orientation="vertical"
              className="flex w-2 touch-none select-none p-0.5"
            >
              <ScrollArea.Thumb className="relative flex-1 rounded-full bg-border" />
            </ScrollArea.Scrollbar>
          </ScrollArea.Root>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

Select.displayName = 'Select';

export { Select, selectTriggerVariants };
export type { SelectOption, SelectProps };
