// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlayerPlay } from '@tabler/icons-react';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import type { VM } from '../../types';
import { SaveActions } from './SaveActions';
import type { VmSettingsForm } from './useVmSettingsForm';

export function StartupSection({ vm, form, className }: { vm: VM; form: VmSettingsForm; className?: string }) {
  const { t } = useTranslation('vm');
  const { draft, set } = form;
  const locked = form.locked('startup');

  return (
    <SettingsGroup
      className={className}
      icon={IconPlayerPlay}
      title={t('settings.sections.startup')}
      description={t('settings.sections.startupDescription')}
      scope={locked ? t('settings.needsStop') : undefined}
      actions={<SaveActions vm={vm} form={form} group="startup" />}
    >
      <div className="grid gap-x-6 gap-y-5 sm:grid-cols-2 lg:grid-cols-3">
        <SettingsField label={t('settings.fields.autostart')}>
          <Switch
            checked={draft.autostart}
            disabled={locked}
            aria-label={t('settings.fields.autostart')}
            onCheckedChange={(value) => set('autostart', value)}
          />
        </SettingsField>
        <SettingsField
          label={t('settings.fields.autostartOrder')}
          description={t('settings.fields.autostartOrderHint')}
        >
          <NumberInput
            value={draft.autostartOrder}
            min={0}
            className="w-40"
            disabled={locked}
            onChange={(value) => set('autostartOrder', value)}
          />
        </SettingsField>
        <SettingsField label={t('settings.fields.autostartDelaySeconds')}>
          <NumberInput
            value={draft.autostartDelaySeconds}
            min={0}
            step={5}
            className="w-40"
            disabled={locked}
            onChange={(value) => set('autostartDelaySeconds', value)}
          />
        </SettingsField>
      </div>
    </SettingsGroup>
  );
}
