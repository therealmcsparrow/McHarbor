// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';

type ToggleRowProps = {
  id: string;
  title: string;
  description: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
};

type MoveContainerOptionsProps = {
  options: ToggleRowProps[];
};

function ToggleRow({ id, title, description, checked, onCheckedChange }: ToggleRowProps) {
  return (
    <div className="flex items-center justify-between gap-4">
      <Label htmlFor={id} className="cursor-pointer">
        <div className="text-sm font-medium">{title}</div>
        <div className="text-xs text-muted-foreground">{description}</div>
      </Label>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  );
}

export function MoveContainerOptions({ options }: MoveContainerOptionsProps) {
  return (
    <div className="space-y-3">
      {options.map((option) => (
        <ToggleRow key={option.id} {...option} />
      ))}
    </div>
  );
}
