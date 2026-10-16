// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// GatedToggleRow — a labelled switch that renders disabled, with a tooltip
// naming the missing permission, when the caller may not flip it. Mirrors the
// disabled-not-hidden rule the gated Button follows.
//
// Lifted out of `AutoUpdateSettingsCard` when the Updates section split into
// Containers / Agents / McHarbor, because both the McHarbor auto-install card
// and the agent auto-update card need the same row.

import { Switch } from '@resources/components/ui/Switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';

type GatedToggleRowProps = {
  label: string;
  hint: string;
  checked: boolean;
  disabled: boolean;
  permission: string;
  onChange: (value: boolean) => void;
};

export function GatedToggleRow({ label, hint, checked, disabled, permission, onChange }: GatedToggleRowProps) {
  const gate = usePermissionGate(permission);
  const control = (
    <Switch checked={checked} onCheckedChange={onChange} disabled={disabled || gate.denied} aria-label={label} />
  );
  return (
    <div className="flex items-center justify-between gap-4">
      <div>
        <p className="text-sm font-medium text-foreground">{label}</p>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
      {gate.denied ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex">{control}</span>
          </TooltipTrigger>
          <TooltipContent>{gate.reason}</TooltipContent>
        </Tooltip>
      ) : (
        control
      )}
    </div>
  );
}
