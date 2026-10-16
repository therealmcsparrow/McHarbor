// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { PortMappingEntry } from './edit-form';

/** Why an exposed port cannot be removed in the editor; absent when it can. */
export type ExposedPortLock = 'image' | 'mapping';

export type ExposedPortRow = { port: string; lockedBy?: ExposedPortLock };

function mappingPort(mapping: PortMappingEntry): string | null {
  const port = mapping.containerPort.trim();
  return port ? `${port}/${mapping.protocol || 'tcp'}` : null;
}

/**
 * The ports a save will expose, and which of them can go. An image's own EXPOSE ports are merged
 * back by the Docker daemon on every create, and a mapping's port is re-added by the save
 * payload, so offering to remove either would be a button that does nothing.
 */
export function exposedPortRows(
  exposed: string[],
  mappings: PortMappingEntry[],
  imagePorts: ReadonlySet<string>,
): ExposedPortRow[] {
  const mapped = new Set(mappings.map(mappingPort).filter((p): p is string => p !== null));
  const all = Array.from(new Set([...exposed, ...mapped])).sort((a, b) =>
    a.localeCompare(b, undefined, { numeric: true }),
  );
  return all.map((port) => {
    if (imagePorts.has(port)) return { port, lockedBy: 'image' };
    if (mapped.has(port)) return { port, lockedBy: 'mapping' };
    return { port };
  });
}

/** Drops a port from the exposed list, unless the image declares it. */
export function removeExposedPort(exposed: string[], port: string, imagePorts: ReadonlySet<string>): string[] {
  if (imagePorts.has(port)) return exposed;
  return exposed.filter((p) => p !== port);
}

/**
 * Releases the port a removed mapping exposed. It stays when the image declares it or another
 * remaining mapping still publishes the same container port.
 */
export function releaseMappingPort(
  exposed: string[],
  removed: PortMappingEntry,
  remaining: PortMappingEntry[],
  imagePorts: ReadonlySet<string>,
): string[] {
  const port = mappingPort(removed);
  if (!port || imagePorts.has(port)) return exposed;
  if (remaining.some((m) => mappingPort(m) === port)) return exposed;
  return exposed.filter((p) => p !== port);
}
