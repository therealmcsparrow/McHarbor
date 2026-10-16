// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconCheck, IconPlus } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { PlainButton } from './PlainButton';
import type { MultiSelectOption } from './MultiSelect';

type AddCustomRowProps = { label: string; onAdd: () => void };

export function AddCustomRow({ label, onAdd }: AddCustomRowProps) {
  return (
    <PlainButton
      // oxlint-disable-next-line jsx-a11y/prefer-tag-over-role -- a custom searchable multi-select; a native <select multiple> cannot host the search field or the add-custom row
      role="option"
      aria-selected={false}
      onClick={onAdd}
      className="flex w-full items-center gap-x-2 rounded-md px-2 py-1.5 text-left text-sm text-foreground transition-colors hover:bg-muted/50"
    >
      <span
        aria-hidden="true"
        className="flex size-4 shrink-0 items-center justify-center rounded border border-dashed border-border"
      >
        <IconPlus className="size-3" />
      </span>
      <span className="min-w-0 flex-1 truncate">{label}</span>
    </PlainButton>
  );
}

type MultiSelectOptionRowProps = {
  option: MultiSelectOption;
  isSelected: boolean;
  onToggle: (value: string) => void;
};

export function MultiSelectOptionRow({ option, isSelected, onToggle }: MultiSelectOptionRowProps) {
  return (
    <PlainButton
      // oxlint-disable-next-line jsx-a11y/prefer-tag-over-role -- a custom searchable multi-select; a native <select multiple> cannot host the search field or the add-custom row
      role="option"
      aria-selected={isSelected}
      disabled={option.disabled}
      title={option.disabled ? option.disabledReason : undefined}
      onClick={() => onToggle(option.value)}
      className={cn(
        'flex w-full items-start gap-x-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors',
        option.disabled ? 'cursor-not-allowed opacity-50' : 'hover:bg-muted/50',
        isSelected ? 'bg-muted/70 text-foreground' : 'text-foreground',
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          'mt-0.5 flex size-4 shrink-0 items-center justify-center rounded border',
          isSelected ? 'border-primary bg-primary text-primary-foreground' : 'border-border',
        )}
      >
        {isSelected && <IconCheck className="size-3" />}
      </span>
      <span className="min-w-0 flex-1">
        <span className={cn('block truncate', isSelected && 'font-medium')}>{option.label}</span>
        {option.description && (
          <span className="block truncate text-xs text-muted-foreground">{option.description}</span>
        )}
      </span>
    </PlainButton>
  );
}
