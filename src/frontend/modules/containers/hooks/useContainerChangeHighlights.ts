// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import type { ContainerInfo } from '@core/types/docker';

type ContainerUpdateResult = {
  containerId: string;
  updateAvailable: boolean;
  error?: string;
};

const HIGHLIGHT_MS = 1800;

function buildContainerSignature(
  container: ContainerInfo,
  updateResult: ContainerUpdateResult | undefined,
): string {
  return JSON.stringify({
    names: container.Names,
    image: container.Image,
    state: container.State,
    status: container.Status,
    ports: container.Ports,
    labels: container.Labels,
    networks: container.NetworkSettings?.Networks,
    update: updateResult
      ? {
          updateAvailable: updateResult.updateAvailable,
          error: updateResult.error ?? '',
        }
      : null,
  });
}

type UseContainerChangeHighlightsOptions = {
  containers: ContainerInfo[];
  updateResults: Map<string, ContainerUpdateResult> | undefined;
  enabled: boolean;
};

export function useContainerChangeHighlights({
  containers,
  updateResults,
  enabled,
}: UseContainerChangeHighlightsOptions) {
  const previousRef = useRef(new Map<string, string>());
  const timeoutsRef = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  const [highlightedIds, setHighlightedIds] = useState<Set<string>>(new Set());

  useEffect(
    () => () => {
      for (const timeout of timeoutsRef.current.values()) {
        clearTimeout(timeout);
      }
    },
    [],
  );

  useEffect(() => {
    const next = new Map<string, string>();
    const changedIds: string[] = [];

    for (const container of containers) {
      const signature = buildContainerSignature(container, updateResults?.get(container.Id));
      next.set(container.Id, signature);
      const previous = previousRef.current.get(container.Id);
      if (enabled && previous && previous !== signature) {
        changedIds.push(container.Id);
      }
    }

    previousRef.current = next;

    if (!enabled) {
      for (const timeout of timeoutsRef.current.values()) {
        clearTimeout(timeout);
      }
      timeoutsRef.current.clear();
      setHighlightedIds(new Set());
      return;
    }

    if (changedIds.length === 0) {
      return;
    }

    setHighlightedIds((current) => {
      const nextSet = new Set(current);
      for (const id of changedIds) {
        nextSet.add(id);
      }
      return nextSet;
    });

    for (const id of changedIds) {
      const existingTimeout = timeoutsRef.current.get(id);
      if (existingTimeout) {
        clearTimeout(existingTimeout);
      }

      const timeout = setTimeout(() => {
        setHighlightedIds((current) => {
          const nextSet = new Set(current);
          nextSet.delete(id);
          return nextSet;
        });
        timeoutsRef.current.delete(id);
      }, HIGHLIGHT_MS);

      timeoutsRef.current.set(id, timeout);
    }
  }, [containers, enabled, updateResults]);

  return highlightedIds;
}
