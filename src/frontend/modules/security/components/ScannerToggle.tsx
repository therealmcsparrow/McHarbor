// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Badge } from "@resources/components/ui/Badge";
import { Switch } from "@resources/components/ui/Switch";

export function ScannerToggle({
  label,
  description,
  checked,
  available,
  onChange,
  t,
}: {
  label: string;
  description: string;
  checked: boolean;
  available?: boolean;
  onChange: (val: boolean) => void;
  t: (key: string) => string;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex items-start gap-3">
        <Switch
          checked={checked}
          aria-label={label}
          className="mt-0.5"
          onCheckedChange={onChange}
        />
        <div>
          <span className="text-sm font-medium text-foreground">{label}</span>
          <p className="text-xs text-muted-foreground">{description}</p>
        </div>
      </div>
      {available !== undefined && (
        <Badge
          variant={available ? "success" : "secondary"}
          className="shrink-0"
        >
          {available ? t("scanning.available") : t("scanning.unavailable")}
        </Badge>
      )}
    </div>
  );
}
