// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// STATUS_PALETTE — central mapping from semantic tone → theme tokens.
//
// This file's shape, exports and helpers are unchanged. What changed is
// what the values resolve to. They used to emit Tailwind palette classes
// (`bg-emerald-500`, `text-amber-300`) which no theme file could
// override — the doc comment claimed the colours came "from the OKLCH
// theme" and they did not. They do now.
//
// Every value below resolves through a token defined in `app.css` and
// overridable by any file under `public/themes/`:
//
//   --status-success / -warning / -danger / -critical / -info / -neutral
//   (each with a readable `-fg` for solid fills)
//   --chart-1 … --chart-8
//
// A user on Dracula gets Dracula's idea of "warning", not Tailwind's amber.
//
// The `dark:` variants that used to appear throughout are gone: the token
// already differs per mode, so restating it was one more thing to keep in
// sync and one more way to drift.
//
// Each Tone carries:
//   * `solid`  — fill + foreground for solid backgrounds (badges
//     with strong emphasis, e.g. destructive buttons).
//   * `soft`   — translucent fill + readable text for the most
//     common badge style (StatusBadge, EventSeverity, etc.).
//   * `outline`— border + readable text for outlined pills.
//   * `dot`    — 2x2 dot colour used as a state indicator.
//   * `text`   — text colour matching the soft background.
//   * `cssVar` — the raw `var(--…)` reference, for consumers that need a
//     colour value rather than a class (SVG charts, canvas, inline gradients).
//
// Tailwind classes are kept fully resolved so consumers can pass
// them directly to `cn()` or `className` without runtime toggles.

import type { ComponentType } from 'react';
import { IconAlertOctagon, IconAlertTriangle, IconBolt, IconCircleCheck, IconInfoCircle } from '@tabler/icons-react';

export type StatusTone = 'success' | 'warning' | 'danger' | 'critical' | 'info' | 'neutral';

export type StatusToken = {
  solid: string;
  soft: string;
  outline: string;
  dot: string;
  /** Text-colour class matching the soft background. */
  text: string;
  /** Raw CSS custom-property reference, for non-class consumers. */
  cssVar: string;
};

export const STATUS_COLORS: Record<StatusTone, StatusToken> = {
  success: {
    solid: 'bg-success text-success-foreground',
    soft: 'bg-success/15 text-success',
    outline: 'border border-success/40 text-success',
    dot: 'bg-success',
    text: 'text-success',
    cssVar: 'var(--status-success)',
  },
  warning: {
    solid: 'bg-warning text-warning-foreground',
    soft: 'bg-warning/15 text-warning',
    outline: 'border border-warning/40 text-warning',
    dot: 'bg-warning',
    text: 'text-warning',
    cssVar: 'var(--status-warning)',
  },
  danger: {
    solid: 'bg-danger text-danger-foreground',
    soft: 'bg-danger/15 text-danger',
    outline: 'border border-danger/40 text-danger',
    dot: 'bg-danger',
    text: 'text-danger',
    cssVar: 'var(--status-danger)',
  },
  // Critical — louder than danger. Used for must-acknowledge events
  // (audit failures, security breaches, lost agent connections).
  // Distinct token + ring so a "critical" entry reads as more urgent
  // than a regular "error" entry without going full destructive.
  critical: {
    solid: 'bg-critical text-critical-foreground ring-1 ring-critical/60',
    soft: 'bg-critical/20 text-critical ring-1 ring-critical/40',
    outline: 'border border-critical/60 text-critical ring-1 ring-critical/30',
    dot: 'bg-critical',
    text: 'text-critical',
    cssVar: 'var(--status-critical)',
  },
  info: {
    solid: 'bg-info text-info-foreground',
    soft: 'bg-info/15 text-info',
    outline: 'border border-info/40 text-info',
    dot: 'bg-info',
    text: 'text-info',
    cssVar: 'var(--status-info)',
  },
  neutral: {
    solid: 'bg-neutral text-neutral-foreground',
    soft: 'bg-neutral/15 text-neutral',
    outline: 'border border-neutral/40 text-neutral',
    dot: 'bg-neutral',
    text: 'text-neutral',
    cssVar: 'var(--status-neutral)',
  },
};

// Convenience helpers — keep callsites short and stop hand-rolling
// repeated Tailwind chains.
export function statusSoft(tone: StatusTone, extra?: string): string {
  return [STATUS_COLORS[tone].soft, extra].filter(Boolean).join(' ');
}

export function statusSolid(tone: StatusTone, extra?: string): string {
  return [STATUS_COLORS[tone].solid, extra].filter(Boolean).join(' ');
}

export function statusOutline(tone: StatusTone, extra?: string): string {
  return [STATUS_COLORS[tone].outline, extra].filter(Boolean).join(' ');
}

export function statusDot(tone: StatusTone): string {
  return STATUS_COLORS[tone].dot;
}

/** The raw colour value for a tone — SVG `fill`, canvas, gradients. */
export function statusVar(tone: StatusTone): string {
  return STATUS_COLORS[tone].cssVar;
}

// ── Chart ramp ────────────────────────────────────────────────────
//
// Eight steps, assigned in fixed order and never cycled. The default
// ramp in `app.css` was validated rather than picked by eye —
// lightness band, chroma floor, adjacent-pair colour-vision
// separation, and contrast against the surface, in both modes. A theme
// may override any step; `scripts/theme-lint.mjs` re-validates whatever
// a theme supplies.

export type ChartStep = 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8;

export const CHART_STEPS: readonly ChartStep[] = [1, 2, 3, 4, 5, 6, 7, 8];

/** `var(--chart-n)` — for SVG/canvas consumers that need a value. */
export function chartVar(step: ChartStep): string {
  return `var(--chart-${step})`;
}

/**
 * Colour for the nth series in a chart. Assignment is positional and
 * stable: series 0 is always step 1, so the same metric keeps the same
 * colour across renders and across widgets.
 */
export function chartSeriesVar(index: number): string {
  const step = CHART_STEPS[Math.abs(index) % CHART_STEPS.length] ?? 1;
  return chartVar(step);
}

export * from './driver-palette';

// ── Severity levels ───────────────────────────────────────────────

export type SeverityLevel = 'info' | 'success' | 'warning' | 'error' | 'critical';

export type SeverityIconProps = {
  className?: string;
  role?: string;
  'aria-label'?: string;
  'aria-hidden'?: boolean | 'true' | 'false';
};

export type SeverityDescriptor = {
  level: SeverityLevel;
  tone: StatusTone;
  icon: ComponentType<SeverityIconProps>;
};

export const SEVERITY_LEVELS: Record<SeverityLevel, SeverityDescriptor> = {
  info: { level: 'info', tone: 'info', icon: IconInfoCircle },
  success: { level: 'success', tone: 'success', icon: IconCircleCheck },
  warning: { level: 'warning', tone: 'warning', icon: IconAlertTriangle },
  error: { level: 'error', tone: 'danger', icon: IconAlertOctagon },
  critical: { level: 'critical', tone: 'critical', icon: IconBolt },
};

export function severityDescriptor(level: string): SeverityDescriptor {
  return (
    SEVERITY_LEVELS[level as SeverityLevel] ?? {
      level: 'info',
      tone: 'neutral',
      icon: IconInfoCircle,
    }
  );
}

// ── Severity helpers ──────────────────────────────────────────────
//
// Same shape as the `status*` helpers above but driven by the
// 5-level severity vocabulary (info / success / warning / error /
// critical). Use these in alerts, audit, lifecycle events, and any
// other surface where the user needs to read at a glance whether
// something needs their attention.

export function severitySoft(level: SeverityLevel | (string & {}), extra?: string): string {
  const descriptor = severityDescriptor(level);
  return [STATUS_COLORS[descriptor.tone].soft, extra].filter(Boolean).join(' ');
}

export function severitySolid(level: SeverityLevel | (string & {}), extra?: string): string {
  const descriptor = severityDescriptor(level);
  return [STATUS_COLORS[descriptor.tone].solid, extra].filter(Boolean).join(' ');
}

export function severityOutline(level: SeverityLevel | (string & {}), extra?: string): string {
  const descriptor = severityDescriptor(level);
  return [STATUS_COLORS[descriptor.tone].outline, extra].filter(Boolean).join(' ');
}

export function severityDot(level: SeverityLevel | (string & {})): string {
  return STATUS_COLORS[severityDescriptor(level).tone].dot;
}

export function severityText(level: SeverityLevel | (string & {})): string {
  return STATUS_COLORS[severityDescriptor(level).tone].text;
}

export function severityVar(level: SeverityLevel | (string & {})): string {
  return STATUS_COLORS[severityDescriptor(level).tone].cssVar;
}
