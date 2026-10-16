// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import type { VM } from '../types';
import { DisplayProfileSection } from './vm-settings/DisplayProfileSection';
import { FeaturesSection } from './vm-settings/FeaturesSection';
import { IdentitySection } from './vm-settings/IdentitySection';
import { PlatformSection } from './vm-settings/PlatformSection';
import { ResourcesSection } from './vm-settings/ResourcesSection';
import { StartupSection } from './vm-settings/StartupSection';
import { useVmSettingsForm } from './vm-settings/useVmSettingsForm';

export function SettingsTab({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const form = useVmSettingsForm(vm);

  return (
    <div className="space-y-5">
      {vm.state === 'unknown' && <Alert variant="warning">{t('settings.unknownNote')}</Alert>}

      {/* Two independent stacks rather than one grid, so a tall left card never stretches the
          row it shares with a short right card. */}
      <div className="grid gap-5 xl:grid-cols-2 xl:items-start">
        <div className="space-y-5">
          <IdentitySection vm={vm} form={form} />
          <StartupSection vm={vm} form={form} />
          <ResourcesSection vm={vm} form={form} />
        </div>
        <div className="space-y-5">
          <FeaturesSection vm={vm} form={form} />
          <PlatformSection vm={vm} form={form} />
          <DisplayProfileSection vm={vm} form={form} />
        </div>
      </div>
    </div>
  );
}
