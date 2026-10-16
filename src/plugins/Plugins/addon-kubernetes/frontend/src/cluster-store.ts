// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Which cluster the pages are reading.
//
// This used to be `kubernetesId` on the host's environment Zustand store,
// deliberately separate from the topbar's Docker host because those are
// different things and the Docker one has no Kubernetes API to answer with.
// The separation survives the move; the storage does not — a plugin has no
// business writing into a host store, and the host store no longer has the
// field.
//
// Implemented on `useSyncExternalStore` rather than Zustand so the bundle
// takes on no dependency for eleven lines of state. The selection persists to
// localStorage, so a reload lands on the cluster the reader was last looking
// at rather than resetting to the default one.

import { useSyncExternalStore } from 'react';

const STORAGE_KEY = 'mcharbor-plugin-kubernetes:cluster';

let current = read();
const listeners = new Set<() => void>();

function read(): string {
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? '';
  } catch {
    // Private mode, or storage disabled by policy. An in-memory selection is
    // still a working one; it just does not survive the reload.
    return '';
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** The selected cluster id, or `''` for "whatever the server calls default". */
export function useSelectedCluster(): string {
  return useSyncExternalStore(
    subscribe,
    () => current,
    () => current,
  );
}

export function setSelectedCluster(id: string): void {
  if (id === current) return;
  current = id;
  try {
    window.localStorage.setItem(STORAGE_KEY, id);
  } catch {
    // Non-fatal, see read().
  }
  listeners.forEach((listener) => listener());
}

/**
 * Query-string params for a cluster-scoped request. Undefined when nothing is
 * selected, so the server resolves its own default rather than being handed an
 * empty string to interpret.
 */
export function clusterParams(id: string): { cluster: string } | undefined {
  return id ? { cluster: id } : undefined;
}

/** The same thing as a suffix, for the endpoints that take a body. */
export function clusterSuffix(id: string): string {
  return id ? `?cluster=${encodeURIComponent(id)}` : '';
}
