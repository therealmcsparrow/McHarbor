// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEnvironmentStore } from '@resources/stores/environment';

/**
 * Publishes the selected environment on `window.McharborHost.environment`.
 *
 * A plugin bundle compiles its own copy of every module it imports, and
 * `@resources/stores/environment` is a module-level Zustand singleton rather
 * than a React context — so a plugin that imported it directly would get a
 * *second* store. It would hydrate from the same `localStorage` key and
 * therefore look correct on first paint, which is the worst possible failure
 * mode: the plugin would show the right environment until the user switched,
 * and then quietly keep answering for the old one. Every request the Swarm
 * add-on makes is scoped by `?env=`, so that is a plugin showing one cluster's
 * nodes under another cluster's name.
 *
 * The shape mirrors the permissions bridge deliberately: `snapshot()` for the
 * value now, `subscribe()` for every value after. Zustand already provides
 * both, so this is a re-export across the bundle boundary rather than a second
 * source of truth — there is exactly one store, and it is the host's.
 *
 * Called from `main.tsx` before the router mounts, so a plugin bundle loaded on
 * the very first route already finds it.
 */
export function publishEnvironmentBridge(): void {
  window.McharborHost = {
    ...window.McharborHost,
    environment: {
      snapshot: () => useEnvironmentStore.getState().currentId,
      subscribe: (listener) =>
        useEnvironmentStore.subscribe((state, prev) => {
          // Zustand fires on every state change, including the environment
          // list refreshing. Plugins care about the selection, so the
          // comparison happens here rather than in each of them.
          if (state.currentId !== prev.currentId) listener(state.currentId);
        }),
      // The list, for plugins that choose an environment rather than follow
      // the host's selection — a guest is created *on* a host, so Compute's
      // create dialog has to name the candidates. Mapped down to id, name and
      // connection type on the way out: a plugin has no business with an
      // environment's connection details or its per-environment settings.
      list: () =>
        useEnvironmentStore.getState().environments.map((env) => ({
          id: env.id,
          name: env.name,
          connectionType: env.connectionType,
        })),
      subscribeList: (listener) =>
        useEnvironmentStore.subscribe((state, prev) => {
          // Reference equality is enough and is what the store actually does:
          // `setEnvironments` replaces the array. Comparing contents would
          // cost a deep compare on every unrelated state change.
          if (state.environments !== prev.environments) {
            listener(
              state.environments.map((env) => ({
                id: env.id,
                name: env.name,
                connectionType: env.connectionType,
              })),
            );
          }
        }),
    },
  };
}
