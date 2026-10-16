// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import type { BreadcrumbSibling } from '@resources/layout/Breadcrumb';
import { deriveEnvironmentStatus, useEnvironmentList } from './useEnvironmentActions';

// The breadcrumb leaf doubles as a switcher between environments, with a
// status dot per peer — the same affordance the container page has.
export function useEnvironmentSiblings(): BreadcrumbSibling[] {
  const { t } = useTranslation('environments');
  const { data: peers = [] } = useEnvironmentList();

  return useMemo(
    () =>
      peers.map((peer) => {
        const peerStatus = deriveEnvironmentStatus(peer);
        return {
          id: peer.id,
          label: peer.name,
          to: `/environments/${peer.id}`,
          adornment: (
            <StatusIndicator
              tone={peerStatus === 'connected' ? 'success' : 'danger'}
              variant="dot"
              size="sm"
              ariaLabel={t(`detail.status.${peerStatus === 'connected' ? 'connected' : 'disconnected'}`)}
            />
          ),
        };
      }),
    [peers, t],
  );
}
