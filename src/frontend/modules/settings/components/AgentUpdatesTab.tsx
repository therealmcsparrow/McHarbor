// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// AgentUpdatesTab — the Updates > Agents section: how remote agent binaries are
// kept in step with the host.

import { AgentUpdateSettingsCard } from './AgentUpdateSettingsCard';
import { AgentInventoryCard } from './AgentInventoryCard';
import { AgentReleasesCard } from './AgentReleasesCard';
import { AgentReleaseSettingsCard } from './AgentReleaseSettingsCard';

export function AgentUpdatesTab() {
  return (
    <div className="space-y-6">
      <AgentUpdateSettingsCard />
      <AgentInventoryCard />
      <AgentReleasesCard />
      <AgentReleaseSettingsCard />
    </div>
  );
}
