// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Tone tables shared by every widget-kit primitive. Whole class strings so
// Tailwind can see them.

export type Tone = 'success' | 'warning' | 'danger' | 'critical' | 'info' | 'neutral' | 'primary';

/** Soft (tinted) surface + matching text, as whole classes for Tailwind. */
export const TONE_SOFT: Record<Tone, string> = {
  success: 'bg-success/15 text-success',
  warning: 'bg-warning/15 text-warning',
  danger: 'bg-danger/15 text-danger',
  critical: 'bg-critical/15 text-critical',
  info: 'bg-info/15 text-info',
  neutral: 'bg-neutral/15 text-neutral',
  primary: 'bg-primary/15 text-primary',
};

/** Solid fill, for bars and dots. */
export const TONE_SOLID: Record<Tone, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  critical: 'bg-critical',
  info: 'bg-info',
  neutral: 'bg-neutral',
  primary: 'bg-primary',
};

/** Text only. */
export const TONE_TEXT: Record<Tone, string> = {
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
  critical: 'text-critical',
  info: 'text-info',
  neutral: 'text-muted-foreground',
  primary: 'text-primary',
};
