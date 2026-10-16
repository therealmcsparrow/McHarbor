// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { i18n as I18n } from 'i18next';
import type { CurrentUserPermissions } from '@core/auth/useCurrentUserPermissions';
import type { ErrorDetails } from '@resources/utils/api-mutation';

/**
 * `window.McharborHost` — the services the host hands to plugin UMD bundles.
 *
 * A plugin bundle compiles its own copy of React, react-i18next and TanStack
 * Query (the host ships ES modules, so it cannot hand them over as UMD
 * globals). Anything that lives in a React context or a module-level singleton
 * therefore does not cross the boundary, and each service that needs to be
 * shared has to be published here explicitly.
 *
 * The declaration lives in one file so the publishers — `core/i18n/i18n.ts`,
 * `core/auth/permissionsBridge.ts` and `core/plugins/environmentBridge.ts` —
 * cannot drift into conflicting shapes for the same global.
 */
export type PermissionsBridge = {
  /** The host's current permission set. Never null; empty before the fetch lands. */
  snapshot: () => CurrentUserPermissions;
  /** Fires on every change. Returns an unsubscribe. */
  subscribe: (listener: (permissions: CurrentUserPermissions) => void) => () => void;
};

/**
 * The host's selected environment, for plugins whose every request is scoped
 * by `?env=`. Published by `environmentBridge.ts`; see that file for why a
 * plugin must not import the store directly.
 */
export type EnvironmentBridge = {
  /** The current environment id. Empty before the environment list lands. */
  snapshot: () => string;
  /** Fires on every change of the selection. Returns an unsubscribe. */
  subscribe: (listener: (environmentId: string) => void) => () => void;
  /**
   * Every environment the host knows, for plugins that present a chooser of
   * their own rather than following the host's selection.
   *
   * Compute is the first: a guest is created *on* a host, so its create dialog
   * has to name the candidates. A plugin cannot read the host's `environments`
   * table — the plugin DB handle is scoped to its own `plugin_<slug>_` prefix —
   * and the only handle that reaches the list on the backend answers with ids
   * alone. Without this a picker would offer raw xids.
   *
   * Empty before the environment list lands, like `snapshot`.
   */
  list: () => EnvironmentSummary[];
  /** Fires when the environment list itself changes. Returns an unsubscribe. */
  subscribeList: (listener: (environments: EnvironmentSummary[]) => void) => () => void;
};

/**
 * The subset of an environment a plugin is given: enough to name one in a
 * picker and to tell an agent host from a socket, and nothing else. Connection
 * details, credentials and per-environment settings stay with the host.
 */
export type EnvironmentSummary = {
  id: string;
  name: string;
  connectionType: string;
};

export type ToastBridge = {
  success: (message: string) => void;
  error: (message: string, details?: ErrorDetails) => void;
  info: (message: string) => void;
  warning: (message: string) => void;
};

export type HostBridge = {
  i18n?: I18n;
  /**
   * Present once the host has mounted. A plugin's `usePermissionGate` reads
   * through this instead of issuing its own `/auth/me/permissions` request, so
   * a grant or revoke reaches plugin UI at the same moment it reaches the host
   * — rather than after the plugin's private 60s staleTime expires.
   */
  permissions?: PermissionsBridge;
  /**
   * Present once the host has mounted. The Swarm add-on is the first plugin
   * whose entire API surface is per-environment; without this it would carry a
   * private copy of the environment store that stops agreeing with the host's
   * the moment the user switches environments.
   */
  environment?: EnvironmentBridge;
  toast?: ToastBridge;
};

declare global {
  interface Window {
    /** Host services handed to plugin UMD bundles. */
    McharborHost?: HostBridge;
  }
}
