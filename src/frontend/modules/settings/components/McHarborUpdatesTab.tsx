// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// McHarborUpdatesTab — the Updates > McHarbor section: the manual version check
// and one-click self-update, the periodic check settings, and whether a found
// release installs itself.

import { PluginStorePointer } from './PluginStorePointer';
import { PrereleaseChannelsCard } from './PrereleaseChannelsCard';
import { McHarborUpdateCard } from './McHarborUpdateCard';
import { SelfUpdateSettingsCard } from './SelfUpdateSettingsCard';
import { AutoUpdateSettingsCard } from './AutoUpdateSettingsCard';
import { UpdateStatusRegion } from './UpdateStatusRegion';
import { UpdateScheduleCard } from './UpdateScheduleCard';

export function McHarborUpdatesTab() {
  return (
    <div className="space-y-6">
      <UpdateStatusRegion />
      <McHarborUpdateCard />
      <SelfUpdateSettingsCard />
      <PrereleaseChannelsCard scope="mcharbor" />
      <UpdateScheduleCard />
      <AutoUpdateSettingsCard />
      <PluginStorePointer />
    </div>
  );
}
