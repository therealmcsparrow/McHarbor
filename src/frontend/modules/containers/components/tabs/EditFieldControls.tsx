// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Switch } from "@resources/components/ui/Switch";

type EditInputProps = {
  label: string;
  value: string | number;
  onChange: (val: string) => void;
  type?: string;
  suffix?: string;
  placeholder?: string;
};

export function EditInput({
  label,
  value,
  onChange,
  type = "text",
  suffix,
  placeholder,
}: EditInputProps) {
  return (
    <div>
      <label className="text-xs font-medium text-muted-foreground">
        {label}
      </label>
      <div className="mt-1 flex items-center gap-2">
        <input
          type={type}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          className="w-full rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
        />
        {suffix && (
          <span className="shrink-0 text-xs text-muted-foreground">
            {suffix}
          </span>
        )}
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

export function ToggleField({
  label,
  checked,
  onChange,
  description,
}: ToggleFieldProps) {
  return (
    <div className="flex items-center justify-between">
      <div>
        <span className="text-xs font-medium text-muted-foreground">
          {label}
        </span>
        {description && (
          <p className="text-[10px] text-muted-foreground/70">{description}</p>
        )}
      </div>
      <Switch checked={checked} aria-label={label} onCheckedChange={onChange} />
    </div>
  );
}
