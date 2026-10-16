// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import type { SeverityLevel } from '@resources/theme/status-palette';
import { cn } from '@resources/utils/cn';

export type SeverityIconProps = {
  level?: SeverityLevel | (string & {});
  severity?: SeverityLevel | (string & {});
  className?: string;
  size?: number | string;
  ariaLabel?: string;
};

// SeverityIcon renders the canonical 5-level severity glyph. The
// component is intentionally presentational (no theme dependency)
// so SeverityBadge, audit log rows, and toast surfaces can share it
// without leaking layout concerns.
export function SeverityIcon(props: SeverityIconProps) {
  const { level, severity, className, size = 16, ariaLabel } = props;
  const toneKey: string = level ?? severity ?? 'info';
  const numericSize = typeof size === 'number' ? size : 16;
  const tone =
    {
      info: 'bg-info/20 text-info',
      success: 'bg-success/20 text-success',
      warning: 'bg-warning/20 text-warning',
      error: 'bg-danger/20 text-danger',
      critical: 'bg-danger/30 text-danger ring-1 ring-danger/40',
    }[toneKey] ?? 'bg-muted text-muted-foreground';

  return (
    <span
      aria-hidden={ariaLabel ? undefined : true}
      className={cn('inline-flex size-(--s) items-center justify-center', tone, className)}
      style={cssVars({ s: `${numericSize}px` })}
    >
      <span aria-hidden="true" className="text-[10px] font-semibold uppercase">
        {toneKey.charAt(0)}
      </span>
      {ariaLabel && <span className="sr-only">{ariaLabel}</span>}
    </span>
  );
}
