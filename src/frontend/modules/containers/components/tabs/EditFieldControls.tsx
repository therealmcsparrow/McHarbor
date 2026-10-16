// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';

type EditInputProps = {
  label: string;
  value: string | number;
  onChange: (val: string) => void;
  type?: string;
  numeric?: boolean;
  suffix?: string;
  placeholder?: string;
};

export function EditInput({
  label,
  value,
  onChange,
  type = 'text',
  numeric = false,
  suffix,
  placeholder,
}: EditInputProps) {
  if (numeric) {
    return (
      <div>
        <label className="text-xs font-medium text-muted-foreground">{label}</label>
        <NumberInput
          nullable
          size="sm"
          aria-label={label}
          value={toNumberOrNull(value)}
          onChange={(next) => onChange(toNumberText(next))}
          placeholder={placeholder}
          suffix={suffix}
          className="mt-1 w-full font-mono"
        />
      </div>
    );
  }
  return (
    <div>
      <label className="text-xs font-medium text-muted-foreground">{label}</label>
      <div className="mt-1 flex items-center gap-2">
        <input
          type={type}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          className="mc-control w-full rounded-md border border-border bg-muted px-2 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
        />
        {suffix && <span className="shrink-0 text-xs text-muted-foreground">{suffix}</span>}
      </div>
    </div>
  );
}

type ToggleFieldProps = {
  label: string;
  checked: boolean;
  onChange: (val: boolean) => void;
  description?: string;
};

export function ToggleField({ label, checked, onChange, description }: ToggleFieldProps) {
  return (
    <div className="flex items-center justify-between">
      <div>
        <span className="text-xs font-medium text-muted-foreground">{label}</span>
        {description && <p className="text-[10px] text-muted-foreground/70">{description}</p>}
      </div>
      <Switch checked={checked} aria-label={label} onCheckedChange={onChange} />
    </div>
  );
}
