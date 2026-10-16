// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTag } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Textarea } from '@resources/components/ui/Textarea';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import type { VM } from '../../types';
import { SaveActions } from './SaveActions';
import type { VmSettingsForm } from './useVmSettingsForm';

export function IdentitySection({ vm, form, className }: { vm: VM; form: VmSettingsForm; className?: string }) {
  const { t } = useTranslation('vm');
  const { draft, set, nameErrorKey } = form;
  const locked = form.locked('identity');

  return (
    <SettingsGroup
      className={className}
      icon={IconTag}
      title={t('settings.sections.identity')}
      description={t('settings.sections.identityDescription')}
      scope={locked ? t('settings.needsStop') : undefined}
      actions={<SaveActions vm={vm} form={form} group="identity" />}
    >
      <div className="grid gap-x-6 gap-y-5 lg:grid-cols-2">
        <SettingsField label={t('settings.fields.name')} hint={nameErrorKey ? t(nameErrorKey) : undefined}>
          <Input
            value={draft.name}
            disabled={locked}
            aria-invalid={nameErrorKey !== null}
            onChange={(evt) => set('name', evt.target.value)}
          />
        </SettingsField>
        <SettingsField label={t('settings.fields.description')}>
          <Textarea
            rows={2}
            value={draft.description}
            disabled={locked}
            onChange={(evt) => set('description', evt.target.value)}
          />
        </SettingsField>
      </div>
    </SettingsGroup>
  );
}
