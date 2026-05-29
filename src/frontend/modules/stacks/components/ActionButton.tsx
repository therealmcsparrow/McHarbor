// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';

type ActionButtonProps = {
  label: string;
  onClick: () => void;
  icon: React.ReactNode;
  disabled?: boolean;
};

export function ActionButton({ label, onClick, icon, disabled = false }: ActionButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          disabled={disabled}
          onClick={(event) => {
            event.stopPropagation();
            if (disabled) return;
            onClick();
          }}
        >
          {icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
