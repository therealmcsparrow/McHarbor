// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconScreenShare } from '@tabler/icons-react';
import { Select } from '@resources/components/ui/Select';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { useDisplayProfiles } from '../../hooks/useDisplayProfiles';
import type { VM } from '../../types';
import { PROFILE_FAST_LAN } from '../../types-display';
import { profileDescription, profileName } from '../displayProfileLabels';
import { SaveActions } from './SaveActions';
import type { VmSettingsForm } from './useVmSettingsForm';

/**
 * The guest's SPICE display profile. Editable in any state: QEMU reads it only
 * at launch, so on a running guest the change waits for the next start.
 */
export function DisplayProfileSection({ vm, form, className }: { vm: VM; form: VmSettingsForm; className?: string }) {
  const { t } = useTranslation('vm');
  const { data: profiles } = useDisplayProfiles();
  const locked = form.locked('display');
  const running = vm.state !== 'stopped';
  const selected = form.draft.displayProfileId || PROFILE_FAST_LAN;

  return (
    <SettingsGroup
      className={className}
      icon={IconScreenShare}
      title={t('displayProfiles.settings.title')}
      description={t('displayProfiles.settings.description')}
      scope={running ? t('displayProfiles.appliesNextStart') : undefined}
      actions={<SaveActions vm={vm} form={form} group="display" />}
    >
      <SettingsField label={t('displayProfiles.settings.label')}>
        <Select
          value={selected}
          ariaLabel={t('displayProfiles.settings.label')}
          disabled={locked || !profiles}
          options={(profiles ?? []).map((profile) => ({
            value: profile.id,
            label: profileName(t, profile),
            description: profileDescription(t, profile),
          }))}
          onChange={(value) => form.set('displayProfileId', value)}
        />
      </SettingsField>
    </SettingsGroup>
  );
}
