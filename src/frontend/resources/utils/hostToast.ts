// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ErrorDetails } from '@resources/utils/api-mutation';

/**
 * The host's toaster, reached across the plugin bundle boundary.
 *
 * A plugin bundle compiles its own `sonner`, and a toast raised through that
 * copy goes onto a store nothing renders — the call returns an id and puts
 * nothing on screen. The host publishes its real toaster on
 * `window.McharborHost.toast` for exactly this reason.
 *
 * Falls back to the console when the bridge is absent, which is the case in a
 * standalone dev harness. Silence is the one option not on the table: a plugin
 * whose failures render nowhere is how a working button looks broken.
 */
export type HostToast = {
  success: (message: string) => void;
  error: (message: string, details?: ErrorDetails) => void;
  info: (message: string) => void;
  warning: (message: string) => void;
};

function bridge(): HostToast | null {
  return (window as Window & { McharborHost?: { toast?: HostToast } }).McharborHost?.toast ?? null;
}

export function createHostToast(tag: string): HostToast {
  return {
    success: (message) => {
      const host = bridge();
      if (host) host.success(message);
      else console.info(`[${tag}] ${message}`);
    },
    error: (message, details) => {
      const host = bridge();
      if (host) host.error(message, details);
      else console.error(`[${tag}] ${message}`, details);
    },
    info: (message) => {
      const host = bridge();
      if (host) host.info(message);
      else console.info(`[${tag}] ${message}`);
    },
    warning: (message) => {
      const host = bridge();
      if (host) host.warning(message);
      else console.warn(`[${tag}] ${message}`);
    },
  };
}
