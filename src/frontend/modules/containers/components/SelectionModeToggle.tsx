// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import type { BackupSelectionMode } from '@resources/hooks/useContainerBackups';

type SelectionModeToggleProps = {
  value: BackupSelectionMode;
  onChange: (mode: BackupSelectionMode) => void;
  // Options that cannot be partially captured (image, logs) hide the
  // middle segment rather than offering a mode that does nothing.
  allowChoose?: boolean;
  // Accessible name for the group, e.g. the option's label.
  label: string;
  disabled?: boolean;
};

// A segmented three-way control: off / choose / full.
//
// It is a radio group rather than three buttons: the three modes are
// mutually exclusive and arrow keys should move between them, which is
// what `role="radiogroup"` buys for free from the platform.
export function SelectionModeToggle({
  value,
  onChange,
  allowChoose = true,
  label,
  disabled = false,
}: SelectionModeToggleProps) {
  const { t } = useTranslation('containers');
  const groupName = useId();

  const modes: { mode: BackupSelectionMode; label: string }[] = [
    { mode: 'off', label: t('backups.mode.off') },
    ...(allowChoose ? [{ mode: 'choose' as BackupSelectionMode, label: t('backups.mode.choose') }] : []),
    { mode: 'full', label: t('backups.mode.full') },
  ];

  return (
    <div
      role="radiogroup"
      aria-label={label}
      className="inline-flex shrink-0 items-center gap-0.5 rounded-lg bg-muted p-0.5"
    >
      {modes.map((entry) => {
        const active = value === entry.mode;
        return (
          <label
            key={entry.mode}
            className={cn(
              'inline-flex h-7 items-center rounded-md px-2.5 text-xs transition-colors',
              'has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring',
              active
                ? 'bg-secondary font-semibold text-foreground'
                : 'font-normal text-muted-foreground hover:bg-muted',
              disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer',
            )}
          >
            <input
              type="radio"
              name={groupName}
              className="sr-only"
              checked={active}
              disabled={disabled}
              onChange={() => onChange(entry.mode)}
            />
            {entry.label}
          </label>
        );
      })}
    </div>
  );
}
