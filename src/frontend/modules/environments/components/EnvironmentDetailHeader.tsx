// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconArrowLeft } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';

type EnvironmentDetailHeaderProps = {
  title: string;
  description?: string;
  backLabel: string;
  onBack: () => void;
  saveLabel?: string;
  onSave?: () => void;
  saveDisabled?: boolean;
  savePending?: boolean;
};

export function EnvironmentDetailHeader({
  title,
  description,
  backLabel,
  onBack,
  saveLabel,
  onSave,
  saveDisabled = false,
  savePending = false,
}: EnvironmentDetailHeaderProps) {
  return (
    <div className="flex flex-1 items-center justify-between gap-4">
      <div className="flex min-w-0 items-center gap-3">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label={backLabel}
              onClick={onBack}
              className="size-8 rounded-lg border border-border text-muted-foreground hover:bg-muted/50 hover:text-foreground"
            >
              <IconArrowLeft className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{backLabel}</TooltipContent>
        </Tooltip>

        <div className="h-5 w-px bg-border" />

        <div className="min-w-0">
          <h1 className="truncate text-sm font-semibold text-foreground">{title}</h1>
          {description ? (
            <p className="truncate text-[11px] leading-tight text-muted-foreground">{description}</p>
          ) : null}
        </div>
      </div>

      {saveLabel && onSave ? (
        <Button size="sm" onClick={onSave} disabled={saveDisabled}>
          {savePending ? <Spinner size="sm" /> : null}
          {saveLabel}
        </Button>
      ) : null}
    </div>
  );
}
