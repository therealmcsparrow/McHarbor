// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * The host's toaster, reached across the bundle boundary.
 *
 * This bundle compiles its own `sonner`, and a toast raised through that copy
 * goes onto a store nothing renders — the call returns an id and puts nothing
 * on screen. The host publishes its real toaster on `window.McharborHost.toast`
 * for exactly this reason.
 *
 * Falls back to the console when the bridge is absent, which is the case in a
 * standalone dev harness. Silence is the one option not on the table: a plugin
 * whose failures render nowhere is how a working button looks broken.
 */
type HostToast = {
  success: (message: string) => void;
  error: (message: string) => void;
  info: (message: string) => void;
  warning: (message: string) => void;
};

function bridge(): HostToast | null {
  return (window as Window & { McharborHost?: { toast?: HostToast } }).McharborHost?.toast ?? null;
}

export const hostToast: HostToast = {
  success: (message) => {
    const host = bridge();
    if (host) host.success(message);
    else console.info(`[compute] ${message}`);
  },
  error: (message) => {
    const host = bridge();
    if (host) host.error(message);
    else console.error(`[compute] ${message}`);
  },
  info: (message) => {
    const host = bridge();
    if (host) host.info(message);
    else console.info(`[compute] ${message}`);
  },
  warning: (message) => {
    const host = bridge();
    if (host) host.warning(message);
    else console.warn(`[compute] ${message}`);
  },
};
