// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef, useState } from 'react';
import type { ContainerInfo } from '@core/types/docker';

type ContainerUpdateResult = {
  containerId: string;
  updateAvailable: boolean;
  error?: string;
};

const HIGHLIGHT_MS = 1800;
const NO_HIGHLIGHTS: Set<string> = new Set();

function buildContainerSignature(container: ContainerInfo, updateResult: ContainerUpdateResult | undefined): string {
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
  const timeoutsRef = useRef(new Map<string, { version: number; timer: ReturnType<typeof setTimeout> }>());
  const signatures = useMemo(() => {
    const next = new Map<string, string>();
    for (const container of containers) {
      next.set(container.Id, buildContainerSignature(container, updateResults?.get(container.Id)));
    }
    return next;
  }, [containers, updateResults]);
  const [previous, setPrevious] = useState(signatures);
  const [highlights, setHighlights] = useState<Map<string, number>>(new Map());

  if (previous !== signatures) {
    setPrevious(signatures);
    if (enabled) {
      const changedIds: string[] = [];
      for (const [id, signature] of signatures) {
        const before = previous.get(id);
        if (before && before !== signature) changedIds.push(id);
      }
      if (changedIds.length > 0) {
        setHighlights((current) => {
          const nextMap = new Map(current);
          for (const id of changedIds) nextMap.set(id, (current.get(id) ?? 0) + 1);
          return nextMap;
        });
      }
    }
  }

  useEffect(() => {
    const timeouts = timeoutsRef.current;
    for (const [id, version] of highlights) {
      const existing = timeouts.get(id);
      if (existing?.version === version) continue;
      if (existing) clearTimeout(existing.timer);
      const timer = setTimeout(() => {
        setHighlights((current) => {
          if (current.get(id) !== version) return current;
          const nextMap = new Map(current);
          nextMap.delete(id);
          return nextMap;
        });
        timeouts.delete(id);
      }, HIGHLIGHT_MS);
      timeouts.set(id, { version, timer });
    }
  }, [highlights]);

  useEffect(
    () => () => {
      for (const { timer } of timeoutsRef.current.values()) {
        clearTimeout(timer);
      }
    },
    [],
  );

  const highlightedIds = useMemo(() => new Set(highlights.keys()), [highlights]);
  return enabled ? highlightedIds : NO_HIGHLIGHTS;
}
