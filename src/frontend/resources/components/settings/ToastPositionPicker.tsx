// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import type { ToastPosition } from '@core/auth/useAuth';

/** The six slots sonner supports, laid out the way they appear on screen. */
const POSITION_ROWS: readonly (readonly ToastPosition[])[] = [
  ['top-left', 'top-center', 'top-right'],
  ['bottom-left', 'bottom-center', 'bottom-right'],
];

/** i18n key for a position, in the `settings` namespace. */
function labelKey(position: ToastPosition): string {
  return `notifications.position.${position.replace('-', '_')}`;
}

type ToastPositionPickerProps = {
  value: ToastPosition;
  onChange: (position: ToastPosition) => void;
  disabled?: boolean;
  /** Permission key the buttons gate on; omit for the per-user picker. */
  permission?: string;
  /** Distinguishes the two pickers' radio groups on the same page. */
  idPrefix: string;
};

/**
 * Picks where toasts appear, as a miniature of the screen rather than a
 * dropdown.
 *
 * A select would read "bottom-center" and make the user translate a phrase
 * into a place. The grid *is* the place: six cells in the shape of a viewport,
 * with the chosen one filled. It costs six buttons and removes the
 * translation step entirely.
 *
 * The grid is 3x2, not 3x3 — sonner has no middle row, and inventing two dead
 * cells to make a nicer square would offer positions that do not exist.
 */
export function ToastPositionPicker({ value, onChange, disabled, permission, idPrefix }: ToastPositionPickerProps) {
  const { t } = useTranslation('settings');

  return (
    <fieldset
      aria-label={t('notifications.positionLabel')}
      className="m-0 grid w-full max-w-sm min-w-0 grid-cols-3 gap-1.5 rounded-lg border border-border bg-background/40 p-1.5"
    >
      {POSITION_ROWS.flat().map((position) => {
        const selected = value === position;
        return (
          <Button
            key={position}
            id={`${idPrefix}-${position}`}
            type="button"
            // Toggle buttons rather than a radiogroup: a real radiogroup owes
            // the user arrow-key navigation and a roving tabindex, and
            // claiming the role without them is worse for a screen reader
            // than not claiming it. aria-pressed says the same thing
            // truthfully for six independently tabbable buttons.
            aria-pressed={selected}
            aria-label={t(labelKey(position))}
            permission={permission}
            variant={selected ? 'default' : 'outline'}
            size="sm"
            disabled={disabled}
            onClick={() => onChange(position)}
            className="h-12 flex-col gap-1 text-[11px] font-normal"
          >
            {/* A dot in the corner the toast will occupy. The button's own
                3x2 place in the grid already says it; the dot repeats it at
                the scale of a single cell so the row reads at a glance. */}
            <span
              aria-hidden="true"
              className={`size-1.5 rounded-full ${selected ? 'bg-primary-foreground' : 'bg-muted-foreground'}`}
            />
            {t(labelKey(position))}
          </Button>
        );
      })}
    </fieldset>
  );
}
