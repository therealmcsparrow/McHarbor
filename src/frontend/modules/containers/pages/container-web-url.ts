// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInspect } from '@core/types/docker';

const WEB_PORTS = new Set([
  80, 443, 8080, 8443, 3000, 3001, 4200, 5000, 5173, 8000, 8888, 8880, 8181, 9000, 9090, 9443,
]);

export function getInspectWebUrl(ports: ContainerInspect['NetworkSettings']['Ports']): string | null {
  if (!ports) return null;

  let bestPort: { hostPort: string; containerPort: number } | null = null;

  for (const [containerPortKey, bindings] of Object.entries(ports)) {
    if (!bindings || bindings.length === 0) continue;
    if (!containerPortKey.endsWith('/tcp')) continue;

    const containerPort = parseInt(containerPortKey);
    if (isNaN(containerPort)) continue;

    const binding = bindings[0];
    if (!binding?.HostPort) continue;

    if (!bestPort || WEB_PORTS.has(containerPort)) {
      bestPort = { hostPort: binding.HostPort, containerPort };
      if (WEB_PORTS.has(containerPort)) break;
    }
  }

  if (!bestPort) return null;

  const scheme =
    bestPort.containerPort === 443 || bestPort.containerPort === 8443 || bestPort.containerPort === 9443
      ? 'https'
      : 'http';
  return `${scheme}://${window.location.hostname}:${bestPort.hostPort}`;
}
