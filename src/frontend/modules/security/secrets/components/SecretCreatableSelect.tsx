// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef, useState } from 'react';
import { PlainButton } from '@resources/components/ui/PlainButton';
import * as Popover from '@radix-ui/react-popover';
import { IconChevronDown, IconPlus, IconX } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';

export type CreatableOption = {
  value: string;
  label: string;
};

type SecretCreatableSelectProps = {
  value: string;
  onChange: (value: string) => void;
  options: CreatableOption[];
  ariaLabel: string;
  placeholder: string;
  searchPlaceholder: string;
  createLabel: (name: string) => string;
  emptyLabel: string;
  clearLabel?: string;
  normalize?: (raw: string) => string;
  maxLength?: number;
};

export function SecretCreatableSelect({
  value,
  onChange,
  options,
  ariaLabel,
  placeholder,
  searchPlaceholder,
  createLabel,
  emptyLabel,
  clearLabel,
  normalize,
  maxLength = 64,
}: SecretCreatableSelectProps) {
  const [isOpen, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  const handleOpenChange = (next: boolean) => {
    if (!next) setQuery('');
    setOpen(next);
  };

  useEffect(() => {
    if (!isOpen) return;
    const timer = window.setTimeout(() => inputRef.current?.focus(), 0);
    return () => window.clearTimeout(timer);
  }, [isOpen]);

  const typed = query.trim();
  const candidate = normalize ? normalize(typed) : typed;

  const filtered = useMemo(() => {
    if (!typed) return options;
    const needle = typed.toLowerCase();
    return options.filter(
      (option) => option.label.toLowerCase().includes(needle) || option.value.toLowerCase().includes(needle),
    );
  }, [options, typed]);

  const canCreate =
    candidate !== '' && !options.some((option) => option.value.toLowerCase() === candidate.toLowerCase());

  const selectedLabel = options.find((option) => option.value === value)?.label ?? value;

  const commit = (next: string) => {
    onChange(next);
    handleOpenChange(false);
  };

  return (
    <Popover.Root open={isOpen} onOpenChange={handleOpenChange}>
      <Popover.Trigger asChild>
        <PlainButton
          aria-label={ariaLabel}
          className="mc-control mc-field mc-field-btn flex w-full items-center justify-between rounded-lg border px-3 text-sm text-foreground transition-colors"
        >
          <span className={cn('truncate', !value && 'text-muted-foreground')}>
            {value ? selectedLabel : placeholder}
          </span>
          <IconChevronDown className="size-4 shrink-0 text-muted-foreground" />
        </PlainButton>
      </Popover.Trigger>

      <Popover.Content
        sideOffset={4}
        align="start"
        className="mc-glass-overlay mc-menu z-50 min-w-[var(--radix-popover-trigger-width)] overflow-hidden rounded-lg"
      >
        <div className="border-b border-border px-3 py-2">
          <input
            ref={inputRef}
            type="text"
            value={query}
            maxLength={maxLength}
            onChange={(eventItem) => setQuery(eventItem.target.value)}
            onKeyDown={(evt) => {
              if (evt.key !== 'Enter' || candidate === '') return;
              evt.preventDefault();
              commit(candidate);
            }}
            placeholder={searchPlaceholder}
            aria-label={searchPlaceholder}
            className="w-full bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
          />
        </div>

        <div className="max-h-60 overflow-y-auto p-1">
          {canCreate && (
            <PlainButton
              onClick={() => commit(candidate)}
              className="flex w-full items-center gap-x-2 rounded-md px-3 py-1.5 text-left text-sm text-foreground transition-colors hover:bg-muted/50"
            >
              <IconPlus className="size-4 shrink-0 text-muted-foreground" />
              <span className="truncate">{createLabel(candidate)}</span>
            </PlainButton>
          )}

          {clearLabel !== undefined && value !== '' && (
            <PlainButton
              onClick={() => commit('')}
              className="flex w-full items-center gap-x-2 rounded-md px-3 py-1.5 text-left text-sm text-foreground transition-colors hover:bg-muted/50"
            >
              <IconX className="size-4 shrink-0 text-muted-foreground" />
              <span className="truncate">{clearLabel}</span>
            </PlainButton>
          )}

          {filtered.map((option) => (
            <PlainButton
              key={option.value}
              onClick={() => commit(option.value)}
              className={cn(
                'flex w-full items-center rounded-md px-3 py-1.5 text-left text-sm transition-colors hover:bg-muted/50',
                option.value === value ? 'bg-muted/70 font-medium text-foreground' : 'text-foreground',
              )}
            >
              <span className="truncate">{option.label}</span>
            </PlainButton>
          ))}

          {filtered.length === 0 && !canCreate && (
            <p className="px-3 py-2 text-sm text-muted-foreground">{emptyLabel}</p>
          )}
        </div>
      </Popover.Content>
    </Popover.Root>
  );
}
