// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { Switch } from '@resources/components/ui/Switch';

type SettingRowProps = {
  title: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
  statusOn: string;
  statusOff: string;
  children?: ReactNode;
};

export function SettingRow({
  title,
  description,
  checked,
  disabled = false,
  onCheckedChange,
  statusOn,
  statusOff,
  children,
}: SettingRowProps) {
  return (
    <div className="space-y-4 rounded-xl border border-border bg-muted/20 p-4">
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <h3 className="text-sm font-semibold text-foreground">{title}</h3>
            <span className="text-[11px] font-semibold tracking-[0.16em] text-muted-foreground uppercase">
              {checked ? statusOn : statusOff}
            </span>
          </div>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>

        <Switch checked={checked} disabled={disabled} onCheckedChange={onCheckedChange} />
      </div>

      {children}
    </div>
  );
}
