// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconSearch } from '@tabler/icons-react';
import * as Popover from '@radix-ui/react-popover';
import * as ScrollArea from '@radix-ui/react-scroll-area';
import { cn } from '@resources/utils/cn';
import { selectTriggerVariants } from './Select';
import { PlainButton } from './PlainButton';
import { AddCustomRow, MultiSelectOptionRow } from './MultiSelect.parts';

type MultiSelectOption = {
  value: string;
  label: string;
  /** Secondary line under the label. Already translated. */
  description?: string;
  /** Renders the row unselectable. */
  disabled?: boolean;
  /** Why the row is unselectable. Already translated; surfaced as the row title. */
  disabledReason?: string;
};

type MultiSelectProps = {
  value: string[];
  onChange: (next: string[]) => void;
  options: MultiSelectOption[];
  placeholder?: string;
  /**
   * Trigger label when nothing is selected AND an empty selection is
   * meaningful rather than missing — "All environments", say. Falls back
   * to `placeholder` when omitted.
   */
  emptyLabel?: string;
  disabled?: boolean;
  className?: string;
  searchable?: boolean;
  ariaLabel?: string;
  portalled?: boolean;
  /**
   * Lets the viewer add a value that is not in `options` by typing it in
   * the search box and pressing Enter (or clicking the "Add" row). Custom
   * values stay visible as selected rows so they can be toggled off again.
   * Implies `searchable`.
   */
  allowCustom?: boolean;
};

/**
 * MultiSelect is the many-of-many sibling of `Select`: one compact trigger
 * that opens a searchable, checkable list. It wears the same trigger
 * material so a form mixing the two reads as one control family.
 *
 * The popover deliberately stays open while rows are toggled — picking
 * several roles should not cost several trips through the trigger.
 *
 * Accessibility: rows are real buttons carrying `role="option"` and
 * `aria-selected` inside a `role="listbox"` with `aria-multiselectable`.
 * A disabled row keeps its reason in `title` so the "why not" is
 * reachable without a hover-only tooltip.
 */
export function MultiSelect({
  value,
  onChange,
  options,
  placeholder,
  emptyLabel,
  disabled,
  className,
  searchable = true,
  ariaLabel,
  portalled = true,
  allowCustom = false,
}: MultiSelectProps) {
  const { t } = useTranslation('common');
  const [isOpen, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const searchRef = useRef<HTMLInputElement>(null);

  const isSearchable = searchable || allowCustom;

  // Custom entries (allowCustom) have no option row of their own, so they
  // are surfaced as synthetic options ahead of the declared list — that
  // keeps them toggleable and counted like any other pick.
  const allOptions = useMemo<MultiSelectOption[]>(() => {
    if (!allowCustom) return options;
    const known = new Set(options.map((option) => option.value));
    const extras: MultiSelectOption[] = value
      .filter((item) => !known.has(item))
      .map((item) => ({ value: item, label: item }));
    return extras.length > 0 ? [...extras, ...options] : options;
  }, [allowCustom, options, value]);

  const selected = useMemo(() => allOptions.filter((option) => value.includes(option.value)), [allOptions, value]);

  const filtered = useMemo(() => {
    if (!search) return allOptions;
    const q = search.toLowerCase();
    return allOptions.filter(
      (option) => option.label.toLowerCase().includes(q) || (option.description?.toLowerCase().includes(q) ?? false),
    );
  }, [allOptions, search]);

  const customCandidate = allowCustom ? search.trim() : '';
  const canAddCustom = customCandidate !== '' && !allOptions.some((option) => option.value === customCandidate);

  useEffect(() => {
    if (isOpen && isSearchable) {
      const id = setTimeout(() => searchRef.current?.focus(), 0);
      return () => clearTimeout(id);
    }
    return undefined;
  }, [isOpen, isSearchable]);

  if (!isOpen && search !== '') setSearch('');

  const toggle = (optionValue: string) => {
    onChange(value.includes(optionValue) ? value.filter((item) => item !== optionValue) : [...value, optionValue]);
  };

  const addCustom = () => {
    if (!canAddCustom) return;
    onChange([...value, customCandidate]);
    setSearch('');
  };

  const resolvedPlaceholder = placeholder ?? t('select.placeholder');
  // One or two picks read better as names; beyond that the count is the
  // honest summary and the list itself carries the detail.
  const triggerLabel =
    selected.length === 0
      ? (emptyLabel ?? resolvedPlaceholder)
      : selected.length <= 2
        ? selected.map((option) => option.label).join(', ')
        : t('select.selectedCount', { count: selected.length });

  const content = (
    <Popover.Content
      sideOffset={4}
      align="start"
      className="mc-glass-overlay mc-menu z-50 max-w-[var(--radix-popover-trigger-width)] min-w-[var(--radix-popover-trigger-width)] overflow-hidden rounded-lg"
    >
      {isSearchable && (
        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <IconSearch className="size-4 shrink-0 text-muted-foreground" />
          <input
            ref={searchRef}
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && canAddCustom) {
                e.preventDefault();
                addCustom();
              }
            }}
            placeholder={allowCustom ? t('select.typeToAddPlaceholder') : t('select.searchPlaceholder')}
            className="w-full bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
          />
        </div>
      )}

      <ScrollArea.Root className="max-h-72 overflow-hidden">
        <ScrollArea.Viewport
          // oxlint-disable-next-line jsx-a11y/prefer-tag-over-role -- a custom searchable multi-select; a native <select multiple> cannot host the search field or the add-custom row
          role="listbox"
          aria-multiselectable="true"
          aria-label={ariaLabel}
          className="max-h-72 w-full p-1"
        >
          {canAddCustom && <AddCustomRow label={t('select.addCustom', { value: customCandidate })} onAdd={addCustom} />}
          {filtered.length === 0 && !canAddCustom ? (
            <div className="px-3 py-2 text-sm text-muted-foreground">
              {allowCustom ? t('select.typeToAdd') : t('select.noResults')}
            </div>
          ) : (
            filtered.map((option) => (
              <MultiSelectOptionRow
                key={option.value}
                option={option}
                isSelected={value.includes(option.value)}
                onToggle={toggle}
              />
            ))
          )}
        </ScrollArea.Viewport>
        <ScrollArea.Scrollbar orientation="vertical" className="flex w-2 touch-none p-0.5 select-none">
          <ScrollArea.Thumb className="relative flex-1 rounded-full bg-border" />
        </ScrollArea.Scrollbar>
      </ScrollArea.Root>

      {selected.length > 0 && (
        <div className="border-t border-border p-1">
          <PlainButton
            onClick={() => onChange([])}
            className="w-full rounded-md px-2 py-1.5 text-left text-sm text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground"
          >
            {t('actions.clear')}
          </PlainButton>
        </div>
      )}
    </Popover.Content>
  );

  return (
    <Popover.Root open={isOpen} onOpenChange={setOpen}>
      <Popover.Trigger asChild disabled={disabled}>
        <PlainButton aria-label={ariaLabel} className={cn(selectTriggerVariants({ variant: 'default' }), className)}>
          <span className={cn('truncate', selected.length === 0 && 'text-muted-foreground')}>{triggerLabel}</span>
          <IconChevronDown
            className={cn('ml-2 size-4 shrink-0 text-muted-foreground transition-transform', isOpen && 'rotate-180')}
          />
        </PlainButton>
      </Popover.Trigger>

      {portalled ? <Popover.Portal>{content}</Popover.Portal> : content}
    </Popover.Root>
  );
}

MultiSelect.displayName = 'MultiSelect';

export type { MultiSelectOption, MultiSelectProps };
