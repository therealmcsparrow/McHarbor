// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { registerShellSlot } from '@resources/layout/shellSlots';
import { LicenseBanner } from '@modules/settings/license/components/LicenseBanner';
import { StandbyBanner } from '@modules/enterprise/components/StandbyBanner';
import { EncryptionKeyBanner } from '@modules/enterprise/components/EncryptionKeyBanner';
import { DemoModeBanner } from '@modules/settings/demo/components/DemoModeBanner';
import { SuppressedInDemo } from '@modules/settings/demo/components/SuppressedInDemo';
import { usePluginNodes } from '@modules/workflows/hooks/usePluginNodes';
import { UpdateCheckBanner } from './UpdateCheckBanner';
import { PluginWidgetBridge } from './pluginWidgetBridge';

const BANNER_CLASS = 'mx-4 mt-3';

// Licensing speaks from the shell rather than from one page: an install that is over its
// entitlement is over it wherever the user is standing. It renders nothing while healthy.
function LicenseShellBanner() {
  return (
    <SuppressedInDemo>
      <LicenseBanner className={BANNER_CLASS} />
    </SuppressedInDemo>
  );
}

function StandbyShellBanner() {
  return <StandbyBanner className={BANNER_CLASS} />;
}

function EncryptionKeyShellBanner() {
  return <EncryptionKeyBanner className={BANNER_CLASS} />;
}

function DemoModeShellBanner() {
  return <DemoModeBanner className={BANNER_CLASS} />;
}

function PluginNodesBridge(): null {
  usePluginNodes();
  return null;
}

registerShellSlot('afterHeader', 'update-check', UpdateCheckBanner);
registerShellSlot('banners', 'license', LicenseShellBanner);
registerShellSlot('banners', 'standby', StandbyShellBanner);
registerShellSlot('banners', 'encryption-key', EncryptionKeyShellBanner);
registerShellSlot('banners', 'demo-mode', DemoModeShellBanner);
registerShellSlot('pluginBridges', 'widgets', PluginWidgetBridge);
registerShellSlot('pluginBridges', 'nodes', PluginNodesBridge);
