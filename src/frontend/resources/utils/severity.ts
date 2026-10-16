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
    dot: 'bg-info',
    pill: 'bg-info/10 ring-1 ring-inset ring-info/30',
    text: 'text-info',
  },
  success: {
    dot: 'bg-success',
    pill: 'bg-success/10 ring-1 ring-inset ring-success/30',
    text: 'text-success',
  },
  warning: {
    dot: 'bg-warning',
    pill: 'bg-warning/10 ring-1 ring-inset ring-warning/30',
    text: 'text-warning',
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
