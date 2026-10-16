// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { appToast, toastErrorDetails } from '@resources/utils/toast';

export type ReconnectOutcome = {
  container: string;
  name: string;
  previousIpv4?: string;
  previousIpv6?: string;
  addressKept: boolean;
  error?: string;
};

export type RecreateNetworkResult = {
  Id: string;
  Warning?: string;
  reconnected: ReconnectOutcome[];
};

/**
 * Says what the recreate did to each container. A moved address is the one outcome the operator
 * must hear about: a service reached by IP, or a DHCP-style reservation on the LAN, breaks quietly.
 */
type Translate = (key: string, options?: Record<string, unknown>) => string;

export function reportRecreateOutcome(results: ReconnectOutcome[], t: Translate): void {
  const failed = results.filter((r) => r.error);
  const moved = results.filter((r) => !r.error && !r.addressKept);
  if (failed.length > 0) {
    toastErrorDetails({
      summary: t('recreate.reconnectFailed', { count: failed.length, names: failed.map((r) => r.name).join(', ') }),
      detail: failed.map((r) => `${r.name}: ${r.error}`).join('\n'),
      at: new Date().toISOString(),
    });
  }
  if (moved.length > 0) {
    appToast.warning(
      t('recreate.addressesChanged', {
        count: moved.length,
        names: moved
          .map((r) => `${r.name} (${[r.previousIpv4, r.previousIpv6].filter(Boolean).join(', ')})`)
          .join(', '),
      }),
    );
  }
  if (failed.length === 0 && moved.length === 0) appToast.success(t('toast.updated'));
}
