// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';

type SecretActionTooltipProps = {
  label: string;
  allowed: boolean;
  children: ReactNode;
};

export function SecretActionTooltip({ label, allowed, children }: SecretActionTooltipProps) {
  if (!allowed) return <>{children}</>;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">{children}</span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
