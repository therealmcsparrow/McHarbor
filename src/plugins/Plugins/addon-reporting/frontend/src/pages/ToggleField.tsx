// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Switch } from '@resources/components/ui/Switch';

/**
 * One labelled switch.
 *
 * The label is a sibling `<label htmlFor>` rather than a wrapper, because
 * Radix's Switch is a `<button>` and nesting an interactive element inside
 * a label makes the click target ambiguous — the browser forwards the
 * click *and* the button handles it, which toggles twice.
 */
export function ToggleField({
  id,
  label,
  checked,
  onToggle,
}: {
  id: string;
  label: string;
  checked: boolean;
  onToggle: () => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <Switch id={id} checked={checked} onCheckedChange={onToggle} aria-label={label} />
      <label htmlFor={id} className="cursor-pointer text-sm text-foreground">
        {label}
      </label>
    </div>
  );
}
