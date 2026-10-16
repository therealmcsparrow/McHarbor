// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { cssVars } from '@resources/utils/cssVars';

/** The cgroup v2 spelling of "no CPU ceiling", shown as the placeholder and the in-force value. */
export const CPU_UNLIMITED = 'max';

/** A usage bar against a limit, or a plain figure where there is no limit. */
export function Meter({
  label,
  value,
  percent,
  caption,
  unbounded = false,
}: {
  label: string;
  value: string;
  percent: number;
  caption?: string;
  unbounded?: boolean;
}) {
  const width = Math.min(100, Math.max(0, Math.round(percent)));
  // Thresholds match the alert defaults, so what the dashboard shows amber and
  // what pages somebody at 3am are the same number.
  const tone = width >= 100 ? 'bg-destructive' : width >= 90 ? 'bg-warning' : 'bg-primary';

  return (
    <div>
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className="mt-0.5 text-sm font-medium text-foreground">{value}</p>
      {/*
        No bar at all where there is no limit. A bar at 0% would read as
        "plenty of headroom" for a container that has no ceiling to have
        headroom against.
      */}
      {unbounded ? (
        <p className="mt-1 text-xs text-muted-foreground">{caption}</p>
      ) : (
        <>
          <div className="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-muted">
            {/*
              The width is a CSS custom property rather than a Tailwind
              arbitrary value: Tailwind generates classes at build time and
              cannot produce `w-[73%]` from a runtime number. This is the same
              `w-(--w)` idiom the host's own ProgressBar uses.
            */}
            <div
              className={`h-full w-(--w) ${tone} transition-all duration-300 ease-out`}
              style={cssVars({ w: `${width}%` })}
            />
          </div>
          {caption && <p className="mt-1 text-xs text-muted-foreground">{caption}</p>}
        </>
      )}
    </div>
  );
}

/** A byte field the operator reads and writes in MB. */
export function BytesField({
  id,
  label,
  hint,
  bytes,
  onChange,
}: {
  id: string;
  label: string;
  hint: string;
  bytes: number | undefined;
  onChange: (bytes: number | undefined) => void;
}) {
  const megabytes = bytes ? Math.round(bytes / 1024 / 1024) : null;

  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      <NumberInput
        id={id}
        nullable
        min={0}
        value={megabytes}
        suffix="MB"
        onChange={(value) => onChange(value && value > 0 ? value * 1024 * 1024 : undefined)}
      />
      <Tooltip>
        <TooltipTrigger asChild>
          <p className="mt-1 cursor-help text-xs text-muted-foreground">{hint}</p>
        </TooltipTrigger>
        <TooltipContent>{hint}</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function share(used: number, limit: number): number {
  if (!limit || limit <= 0) return 0;
  return (used / limit) * 100;
}

export function percentLabel(used: number, limit: number): string {
  return `${Math.round(share(used, limit))}%`;
}
