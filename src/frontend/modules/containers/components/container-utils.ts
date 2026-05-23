// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInfo } from '@core/types/docker';

export const STATE_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  running: 'success',
  exited: 'destructive',
  paused: 'warning',
  restarting: 'warning',
  created: 'secondary',
  removing: 'destructive',
  dead: 'destructive',
};

export const HEALTH_COLORS: Record<string, string> = {
  healthy: 'text-emerald-500',
  unhealthy: 'text-red-500',
  starting: 'text-amber-500',
};

export function parseHealth(status: string): string | null {
  const match = status.match(/\((healthy|unhealthy|starting)\)/);
  return match ? match[1] ?? null : null;
}

export function parseUptime(status: string): string {
  const upMatch = status.match(/^Up\s+(.+?)(\s+\(|$)/);
  if (upMatch?.[1]) {
    const raw = upMatch[1].trim();
    return raw.replace(/^About an?\s+/, '1 ').replace(/Less than a\s+/, '<1 ');
  }
  return '-';
}

export function formatPorts(ports: ContainerInfo['Ports']): string {
  const publicPorts = getPublicPorts(ports);
  return publicPorts.join(', ') || '-';
}

export function getContainerIP(c: ContainerInfo): string {
  if (!c.NetworkSettings?.Networks) return '-';
  const nets = Object.values(c.NetworkSettings.Networks);
  const withIP = nets.find((n) => n.IPAddress);
  return withIP?.IPAddress ?? '-';
}

export function getStackName(c: ContainerInfo): string | null {
  return c.Labels?.['com.docker.compose.project'] ?? null;
}

export function getPublicPorts(ports: ContainerInfo['Ports']): string[] {
  if (!ports) return [];

  const seen = new Set<string>();
  const publicPorts: string[] = [];

  for (const port of ports) {
    if (!port.PublicPort) continue;

    const label = `${port.PublicPort}:${port.PrivatePort}/${port.Type}`;
    if (seen.has(label)) continue;

    seen.add(label);
    publicPorts.push(label);
  }

  return publicPorts;
}

export function getAutoUpdate(c: ContainerInfo): boolean {
  const labels = c.Labels ?? {};
  return (
    labels['com.centurylinklabs.watchtower.enable'] === 'true' ||
    labels['io.containrrr.watchtower.enable'] === 'true' ||
    labels['com.ouroboros.enable'] === 'true' ||
    !!labels['mcharbor.auto-update']
  );
}

const WEB_PORTS = new Set([
  80, 443, 8080, 8443, 3000, 3001, 4200, 5000, 5173, 8000, 8888, 8880, 8181, 9000, 9090, 9443,
]);

export function getContainerWebUrl(ports: ContainerInfo['Ports']): string | null {
  if (!ports || ports.length === 0) return null;
  const tcpPorts = ports.filter((p) => p.PublicPort && p.Type === 'tcp');
  if (tcpPorts.length === 0) return null;
  const webPort = tcpPorts.find((p) => WEB_PORTS.has(p.PrivatePort)) ?? tcpPorts[0];
  if (!webPort?.PublicPort) return null;
  const scheme =
    webPort.PrivatePort === 443 || webPort.PrivatePort === 8443 || webPort.PrivatePort === 9443
      ? 'https'
      : 'http';
  return `${scheme}://${window.location.hostname}:${webPort.PublicPort}`;
}
