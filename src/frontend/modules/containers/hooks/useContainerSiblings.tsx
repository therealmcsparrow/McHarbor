// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useMemo } from 'react';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import type { BreadcrumbSibling } from '@resources/layout/Breadcrumb';
import { containerStatePresentation } from '@resources/utils/container-state';
import { useContainers } from './useContainers';

/**
 * Peers for the breadcrumb leaf switcher, so moving between two containers
 * does not mean a round trip through the list. The list is already cached by
 * the containers page in the common case.
 */
export function useContainerSiblings(): BreadcrumbSibling[] {
  const { data: allContainers = [] } = useContainers();
  return useMemo(
    () =>
      allContainers.map((peer) => {
        const peerName = peer.Names?.[0]?.replace(/^\//, '') ?? peer.Id;
        const presentation = containerStatePresentation(peer.State);
        return {
          id: peer.Id,
          label: peerName,
          to: `/containers/${peer.Id}`,
          adornment: <StatusIndicator tone={presentation.tone} variant="dot" size="sm" ariaLabel={peer.State} />,
        };
      }),
    [allContainers],
  );
}
