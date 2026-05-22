// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Button } from '@resources/components/ui/Button';

type ToggleFieldProps = {
  label: string;
  checked: boolean;
  onChange: (value: boolean) => void;
};

export function ToggleField({ label, checked, onChange }: ToggleFieldProps) {
  return (
    <div className="flex items-center justify-between border-b border-border py-2 last:border-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        role="switch"
        aria-checked={checked}
        className={`relative h-5 w-9 rounded-full p-0 ${checked ? 'bg-primary hover:bg-primary/90' : 'bg-muted-foreground/30 hover:bg-muted-foreground/40'}`}
        onClick={() => onChange(!checked)}
      >
        <span
          className={`pointer-events-none inline-block size-4 rounded-full bg-white shadow-sm transition-transform ${checked ? 'translate-x-2' : '-translate-x-2'}`}
        />
      </Button>
    </div>
  );
}
