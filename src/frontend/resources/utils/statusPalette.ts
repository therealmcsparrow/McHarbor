// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Status palettes for operator-facing surfaces. Kept as a typed
// record so consumers can extend without leaking Tailwind class
// strings to the call site.

export type PaletteTone = {
  label: string;
  swatch: string;
  className: string;
};

// Network driver palette — maps bridge / host / overlay / macvlan /
// ipvlan / none to consistent chip styles. Reused by container
// create dialog, network list rows, and compose stack previews.
// The fallback is a named const rather than a member fished back out of the
// record: a `Record<string, PaletteTone>` index is optional to TypeScript, so
// reading it back needed a non-null assertion to say what the literal already
// showed.
const NETWORK_DRIVER_FALLBACK: PaletteTone = {
  label: 'None',
  swatch: 'bg-muted-foreground',
  className: 'bg-muted text-muted-foreground',
};

export const NETWORK_DRIVER_PALETTE: Record<string, PaletteTone> = {
  bridge: { label: 'Bridge', swatch: 'bg-info', className: 'bg-info/10 text-info' },
  host: { label: 'Host', swatch: 'bg-warning', className: 'bg-warning/10 text-warning' },
  overlay: { label: 'Overlay', swatch: 'bg-chart-3', className: 'bg-chart-3/10 text-chart-3' },
  macvlan: { label: 'MACVLAN', swatch: 'bg-chart-1', className: 'bg-chart-1/10 text-chart-1' },
  ipvlan: { label: 'IPVLAN', swatch: 'bg-success', className: 'bg-success/10 text-success' },
  none: NETWORK_DRIVER_FALLBACK,
};

export function networkDriverClasses(driver: string): string {
  return NETWORK_DRIVER_PALETTE[driver]?.className ?? NETWORK_DRIVER_FALLBACK.className;
}

// Container state palette — running / exited / paused / restarting /
// dead / created / removed. Tone matches the Badge component's
// success / warning / destructive vocabulary.
const CONTAINER_STATE_FALLBACK: PaletteTone = {
  label: 'Removed',
  swatch: 'bg-muted-foreground',
  className: 'bg-muted text-muted-foreground',
};

export const CONTAINER_STATE_PALETTE: Record<string, PaletteTone> = {
  running: { label: 'Running', swatch: 'bg-success', className: 'bg-success/10 text-success' },
  exited: { label: 'Stopped', swatch: 'bg-danger', className: 'bg-danger/10 text-danger' },
  paused: { label: 'Paused', swatch: 'bg-warning', className: 'bg-warning/10 text-warning' },
  restarting: { label: 'Restarting', swatch: 'bg-info', className: 'bg-info/10 text-info' },
  dead: { label: 'Dead', swatch: 'bg-danger', className: 'bg-danger/10 text-danger' },
  created: { label: 'Created', swatch: 'bg-chart-3', className: 'bg-chart-3/10 text-chart-3' },
  removed: CONTAINER_STATE_FALLBACK,
};

export function containerStateClasses(state: string): string {
  return CONTAINER_STATE_PALETTE[state]?.className ?? CONTAINER_STATE_FALLBACK.className;
}

// Health palette — healthy / unhealthy / starting / none.
const HEALTH_FALLBACK: PaletteTone = {
  label: 'No healthcheck',
  swatch: 'bg-muted-foreground',
  className: 'bg-muted text-muted-foreground',
};

export const HEALTH_PALETTE: Record<string, PaletteTone> = {
  healthy: { label: 'Healthy', swatch: 'bg-success', className: 'bg-success/10 text-success' },
  unhealthy: { label: 'Unhealthy', swatch: 'bg-danger', className: 'bg-danger/10 text-danger' },
  starting: { label: 'Starting', swatch: 'bg-warning', className: 'bg-warning/10 text-warning' },
  none: HEALTH_FALLBACK,
};

export function healthClasses(health: string): string {
  return HEALTH_PALETTE[health]?.className ?? HEALTH_FALLBACK.className;
}
