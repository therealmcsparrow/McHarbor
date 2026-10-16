// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The selected environment, read from the host.
//
// Every request this plugin makes is scoped by `?env=`, so "which environment"
// is the single most load-bearing value in the bundle. It cannot come from
// `@resources/stores/environment`: that module is a Zustand singleton, and a
// plugin bundle compiles its own copy of every module it imports. Importing it
// would give the plugin a *second* store that hydrates from the same
// `localStorage` key — correct on first paint, and then silently wrong from
// the moment the user switches environments.
//
// So the host publishes its store on `window.McharborHost.environment`
// (`core/plugins/environmentBridge.ts`) and this reads through it. The export
// is deliberately named and shaped like the store's own hook so the eleven
// hooks that were ported from the host module kept their call sites
// unchanged — `useEnvironmentStore((s) => s.currentId)` reads the same here as
// it did there.

import { useSyncExternalStore } from 'react';

type HostEnvironmentBridge = {
  snapshot: () => string;
  subscribe: (listener: (environmentId: string) => void) => () => void;
};

/** The slice of the host store this plugin can see. */
type EnvironmentState = {
  currentId: string;
};

function bridge(): HostEnvironmentBridge | null {
  return (
    (window as Window & { McharborHost?: { environment?: HostEnvironmentBridge } }).McharborHost?.environment ?? null
  );
}

/**
 * Reads the host's selected environment.
 *
 * Takes a selector, like the store hook it replaces, so the ported hooks did
 * not have to change. The state it selects from has one field because one
 * field is all the host publishes — the environment *list* is the
 * environment picker's business, and the picker is the host's.
 *
 * With no bridge (a plugin loaded outside the host shell, or a host older than
 * the bridge) this returns the empty id. The hooks all pass `?env=` through to
 * the backend, which resolves an empty id to the default environment, so the
 * degraded case is "the default host" rather than a crash.
 */
// The snapshot is re-read on subscribe as well as on change: the bundle may have been
// evaluated before the host's first publish, in which case the first read is a stale empty string.
function subscribeToHost(onChange: () => void): () => void {
  const host = bridge();
  if (!host) return () => undefined;
  return host.subscribe(() => onChange());
}

function currentEnvironmentId(): string {
  return bridge()?.snapshot() ?? '';
}

export function useEnvironmentStore<T>(selector: (state: EnvironmentState) => T): T {
  const currentId = useSyncExternalStore(subscribeToHost, currentEnvironmentId, currentEnvironmentId);
  return selector({ currentId });
}
