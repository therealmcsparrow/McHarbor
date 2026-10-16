// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { HostTogglesSection } from './HostTogglesSection';
import { HostRootsSection } from './HostRootsSection';
import { HostPoolsSection } from './HostPoolsSection';
import { HostNetworksSection } from './HostNetworksSection';
import type { OverviewEntry } from '../../types';

interface HostSettingsTabProps {
  entry: OverviewEntry;
}

export function HostSettingsTab({ entry }: HostSettingsTabProps) {
  return (
    <div className="space-y-4">
      <HostTogglesSection entry={entry} />
      <HostRootsSection entry={entry} />
      <HostPoolsSection entry={entry} />
      <HostNetworksSection entry={entry} />
    </div>
  );
}
