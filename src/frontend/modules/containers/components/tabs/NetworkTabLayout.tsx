// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentProps } from 'react';
import { NetworkTabConfigSection } from './NetworkTabConfigSection';
import { NetworkTabPortsSection } from './NetworkTabPortsSection';

type NetworkTabLayoutProps = {
  layout: 'grid' | 'editor';
  ports: ComponentProps<typeof NetworkTabPortsSection>;
  config: ComponentProps<typeof NetworkTabConfigSection>;
};

/**
 * The configuration editor stacks everything it saves on the left and keeps the live Networks card on
 * the right; the detail page keeps the original two-column grid.
 */
export function NetworkTabLayout({ layout, ports, config }: NetworkTabLayoutProps) {
  if (layout === 'editor') {
    return (
      <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-6">
          <NetworkTabPortsSection {...ports} part="mappings" />
          <NetworkTabConfigSection {...config} parts={['extraHosts', 'dns']} />
          <NetworkTabPortsSection {...ports} part="exposed" />
        </div>
        <div className="min-w-0">
          <NetworkTabConfigSection {...config} parts={['networks']} />
        </div>
      </div>
    );
  }
  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <NetworkTabPortsSection {...ports} />
      <NetworkTabConfigSection {...config} />
    </div>
  );
}
