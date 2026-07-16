// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Shared severity colorization for log / event tables
// (BackupLogsTab, LifecycleLogPage, future log views).
// Every severity value gets a coloured dot + pill that works in
// both light and dark mode without depending on the Badge
// variant tones.

export type SeverityStyle = {
  dot: string;
  pill: string;
  text: string;
};

export const SEVERITY_STYLES: Record<string, SeverityStyle> = {
  info: {
    dot: 'bg-sky-500',
    pill: 'bg-sky-500/10 ring-1 ring-inset ring-sky-500/30',
    text: 'text-sky-700 dark:text-sky-300',
  },
  success: {
    dot: 'bg-emerald-500',
    pill: 'bg-emerald-500/10 ring-1 ring-inset ring-emerald-500/30',
    text: 'text-emerald-700 dark:text-emerald-300',
  },
  warning: {
    dot: 'bg-amber-500',
    pill: 'bg-amber-500/10 ring-1 ring-inset ring-amber-500/30',
    text: 'text-amber-700 dark:text-amber-300',
  },
  error: {
    dot: 'bg-destructive',
    pill: 'bg-destructive/10 ring-1 ring-inset ring-destructive/30',
    text: 'text-destructive',
  },
};

export const DEFAULT_SEVERITY_STYLE: SeverityStyle = {
  dot: 'bg-muted-foreground',
  pill: 'bg-muted ring-1 ring-inset ring-border',
  text: 'text-muted-foreground',
};

export function severityStyle(severity: string): SeverityStyle {
  return SEVERITY_STYLES[severity] ?? DEFAULT_SEVERITY_STYLE;
}