// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { formatDate, type LocaleFormat } from "@resources/utils/format";
import { type ContainerBackupRunDestination } from "../hooks/useContainerBackups";

// formatBytes renders a byte count as a human-readable string with
// binary unit suffixes. Zero and negative values render as "0 B".
export function formatBytes(value: number) {
  if (!value) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length - 1,
  );
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

// formatTimestamp renders an RFC3339 timestamp with the active
// locale and the user's chosen time + date format. Returns an
// em-dash for empty values; `formatDate` already falls back to the
// raw string when the Date constructor throws.
export function formatTimestamp(value?: string, prefs?: Partial<LocaleFormat>) {
  if (!value) return "—";
  const formatted = formatDate(value, prefs);
  return formatted === "-" ? value : formatted;
}

// destinationLabel prefers the human-readable storage location
// name on a backup run destination, then the id, then the
// caller-provided fallback. Used by the runs table when rendering
// the destination column.
export function destinationLabel(
  destination: ContainerBackupRunDestination,
  fallback: string,
) {
  if (destination.storageLocationName) return destination.storageLocationName;
  if (destination.storageLocationId) return destination.storageLocationId;
  return fallback;
}

// statusBadgeVariant maps a backup run status onto the
// shared <Badge> variant so the badge color matches the rest of
// the app.
export function statusBadgeVariant(status: string): "default" | "destructive" | "secondary" | "outline" {
  switch (status) {
    case "running":
      return "outline";
    case "success":
      return "default";
    case "failure":
      return "destructive";
    case "cancelled":
      return "secondary";
    default:
      return "secondary";
  }
}

// environmentName is a placeholder hook for the future when the
// Backups overview might run with a per-row environment id that
// differs from the current global env. Today it always returns the
// fallback because the global env is the only context. Kept as
// a helper so call sites read naturally.
export function environmentName(envId: string | undefined, fallback: string) {
  if (!envId) return fallback;
  return fallback;
}
