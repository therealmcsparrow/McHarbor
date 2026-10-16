// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The two shapes every Docker Info panel is built from, plus the byte
// formatter they share.
//
// These lived as private copies inside `DockerInfoTab`. They moved out
// when the Resources section grew a second contributor
// (`HostHardwareRows`): two components rendering rows into the same
// bordered stack have to agree on the row, and the only way to guarantee
// that is for there to be one row.

import type { ReactNode } from 'react';

/** Human-readable byte size, binary units. */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

type InfoRowProps = {
  label: string;
  value: ReactNode;
};

/**
 * One label/value line. The value is right-aligned and capped at 60% of
 * the row so a long driver string cannot push the label off the left.
 */
export function InfoRow({ label, value }: InfoRowProps) {
  return (
    <div className="flex items-start justify-between border-b border-border py-2 last:border-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="max-w-[60%] text-right text-sm break-all text-foreground">{value}</span>
    </div>
  );
}

type SectionProps = {
  title: string;
  children: ReactNode;
};

/** A titled stack of `InfoRow`s on its own subtle surface. */
export function Section({ title, children }: SectionProps) {
  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold text-foreground">{title}</h3>
      <div className="rounded-lg border border-border bg-muted/30 px-4">{children}</div>
    </div>
  );
}
