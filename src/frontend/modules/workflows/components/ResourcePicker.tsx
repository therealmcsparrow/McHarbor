// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { cn } from '@resources/utils/cn';

export type ResourcePickerItem = {
  value: string;
  title: string;
  subtitle?: string;
  badge?: string;
  tone?: 'default' | 'success' | 'danger';
};

interface ResourcePickerProps {
  value: string;
  onChange: (value: string) => void;
  items: ResourcePickerItem[];
  placeholder: string;
  emptyLabel: string;
  /** Shown instead of the list when a prerequisite (usually the environment) is missing. */
  blockedLabel?: string;
  isLoading?: boolean;
  /** Lets the operator type a value that is not in the list (e.g. a new stack name). */
  allowCustom?: boolean;
  /** Optional first entry that clears the value ("Use default ..."). */
  clearLabel?: string;
  /** The key the list is read with; a denied operator sees the control disabled, naming it. */
  permission?: string;
  /** Why the list cannot be offered at all; the control renders disabled with this beneath it. */
  unavailableReason?: string;
}

/**
 * Shared dropdown for every "pick a McHarbor resource" config field.
 * Items come from the caller's query; this component only owns the
 * open/close state, the custom-value input, and the option rendering.
 */
export function ResourcePicker({
  value,
  onChange,
  items,
  placeholder,
  emptyLabel,
  blockedLabel,
  isLoading,
  allowCustom,
  clearLabel,
  permission,
  unavailableReason,
}: ResourcePickerProps) {
  const { t } = useTranslation('common');
  const [isOpen, setOpen] = useState(false);
  const [customMode, setCustomMode] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    const onPointerDown = (pointerEvent: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(pointerEvent.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onPointerDown);
    return () => document.removeEventListener('mousedown', onPointerDown);
  }, [isOpen]);

  const selected = items.find((item) => item.value === value);
  const isCustomValue = Boolean(value) && !selected;
  const displayLabel = value ? (selected?.title ?? value) : (clearLabel ?? placeholder);
  const loadingLabel = t('workflows.loadingOptions');
  const customLabel = t('workflows.useCustomValue');

  if (allowCustom && (customMode || isCustomValue)) {
    return (
      <div className="flex items-center gap-2">
        <Input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className="text-xs" />
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            setCustomMode(false);
            onChange('');
          }}
        >
          {t('workflows.pickFromList')}
        </Button>
      </div>
    );
  }

  return (
    <div className="relative" ref={rootRef}>
      <Button
        type="button"
        variant="ghost"
        aria-label={placeholder}
        aria-expanded={isOpen}
        permission={permission}
        disabled={Boolean(unavailableReason)}
        onClick={() => setOpen((prev) => !prev)}
        className={cn(
          'flex w-full items-center justify-between rounded-md border border-input bg-card px-2 text-left text-xs ring-offset-background focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
          !value && 'text-muted-foreground',
        )}
      >
        <span className="truncate">{displayLabel}</span>
        <IconChevronDown className="size-3 shrink-0 text-muted-foreground" />
      </Button>
      {unavailableReason ? <p className="mt-1 text-[10px] text-muted-foreground">{unavailableReason}</p> : null}
      {isOpen && (
        <div className="absolute top-full right-0 left-0 z-50 mt-1 max-h-56 overflow-y-auto rounded-md border border-border bg-popover py-1 shadow-xl">
          {clearLabel ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                onChange('');
                setOpen(false);
              }}
              className={cn(
                'flex w-full items-center px-3 py-1.5 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/50',
                !value && 'bg-muted/50 text-foreground',
              )}
            >
              {clearLabel}
            </Button>
          ) : null}
          {blockedLabel ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">{blockedLabel}</p>
          ) : isLoading ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">{loadingLabel}</p>
          ) : items.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">{emptyLabel}</p>
          ) : (
            items.map((item) => (
              <Button
                key={item.value}
                type="button"
                variant="ghost"
                onClick={() => {
                  onChange(item.value);
                  setOpen(false);
                }}
                className={cn(
                  'flex h-auto w-full flex-col items-start gap-0.5 px-3 py-1.5 text-left transition-colors hover:bg-muted/50',
                  item.value === value && 'bg-muted/50',
                )}
              >
                <span className="flex w-full items-center gap-1.5 text-xs font-semibold text-foreground">
                  <span className="truncate">{item.title}</span>
                  {item.badge ? (
                    <span
                      className={cn(
                        'px-1.5 py-0.5 text-[9px] font-medium tracking-wide uppercase',
                        item.tone === 'success' && 'bg-success/10 text-success',
                        item.tone === 'danger' && 'bg-danger/10 text-danger',
                        (!item.tone || item.tone === 'default') && 'bg-primary/10 text-primary',
                      )}
                    >
                      {item.badge}
                    </span>
                  ) : null}
                </span>
                {item.subtitle ? (
                  <span className="w-full truncate text-[10px] text-muted-foreground">{item.subtitle}</span>
                ) : null}
              </Button>
            ))
          )}
          {allowCustom ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                setCustomMode(true);
                setOpen(false);
              }}
              className="flex w-full items-center border-t border-border px-3 py-1.5 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/50"
            >
              {customLabel}
            </Button>
          ) : null}
        </div>
      )}
    </div>
  );
}
