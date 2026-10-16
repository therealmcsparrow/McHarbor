// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInfo } from '@core/types/docker';
import { createSearchMatcher, type SearchMode } from '@resources/utils/search-filter';

type SearchTerm = { field: string | null; value: string; negated: boolean };

function parseTerms(query: string): SearchTerm[] {
  const terms: SearchTerm[] = [];
  const pattern = /(-?)(?:([a-z]+):)?(?:"([^"]*)"|(\S+))/gi;
  for (const match of query.matchAll(pattern)) {
    const value = match[3] ?? match[4] ?? '';
    if (value) {
      terms.push({
        field: match[2]?.toLowerCase() ?? null,
        value,
        negated: match[1] === '-',
      });
    }
  }
  return terms;
}

function valuesFor(container: ContainerInfo, field: string | null): string[] {
  const containerName = container.Names?.[0]?.replace(/^\//, '') ?? container.Id;
  const labels = Object.entries(container.Labels ?? {}).flatMap(([key, value]) => [key, value, `${key}=${value}`]);
  const networks = Object.entries(container.NetworkSettings?.Networks ?? {}).flatMap(([networkName, network]) => [
    networkName,
    network.IPAddress,
    network.GlobalIPv6Address,
    ...(network.Aliases ?? []),
  ]);
  const mounts = (container.Mounts ?? []).flatMap((mount) => [mount.Name ?? '', mount.Source, mount.Destination]);
  const ports = (container.Ports ?? []).flatMap((port) => [
    String(port.PrivatePort),
    String(port.PublicPort ?? ''),
    port.IP ?? '',
    `${port.PublicPort ?? ''}:${port.PrivatePort}/${port.Type}`,
  ]);
  const fields: Record<string, string[]> = {
    name: [containerName, container.Id],
    image: [container.Image, container.ImageID],
    state: [container.State, container.Status],
    stack: [container.StackName ?? '', container.StackService ?? ''],
    network: networks,
    volume: mounts,
    mount: mounts,
    port: ports,
    label: labels,
    protected: [String(Boolean(container.Protected))],
  };
  if (field && fields[field]) return fields[field];
  return Object.values(fields).flat();
}

export function filterContainers(containers: ContainerInfo[], query: string, mode: SearchMode) {
  const terms = parseTerms(query);
  if (terms.length === 0) return { matches: containers, error: null };
  const matchers = terms.map((term) => ({
    ...term,
    matcher: createSearchMatcher(term.value, mode),
  }));
  const error = matchers.find((term) => term.matcher.error)?.matcher.error ?? null;
  if (error) return { matches: containers, error };
  return {
    error: null,
    matches: containers.filter((container) =>
      matchers.every((term) => {
        const matched = valuesFor(container, term.field).some(term.matcher.matches);
        return term.negated ? !matched : matched;
      }),
    ),
  };
}
