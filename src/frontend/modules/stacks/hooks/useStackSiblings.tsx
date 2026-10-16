// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useStacks } from '@resources/hooks/useStacks';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import type { BreadcrumbSibling } from '@resources/layout/Breadcrumb';

const STATUS_TONES: Record<string, 'success' | 'warning' | 'danger'> = {
  running: 'success',
  partial: 'warning',
  stopped: 'danger',
};

/** Peers for the breadcrumb leaf switcher, so moving between two stacks does not mean a round trip through the list. */
export function useStackSiblings(): BreadcrumbSibling[] {
  const { data: allStacks = [] } = useStacks();
  return useMemo(
    () =>
      allStacks.map((peer) => ({
        id: peer.name,
        label: peer.name,
        to: `/stacks/${peer.name}`,
        adornment: (
          <StatusIndicator
            tone={STATUS_TONES[peer.status] ?? 'neutral'}
            variant="dot"
            size="sm"
            ariaLabel={peer.status}
          />
        ),
      })),
    [allStacks],
  );
}
