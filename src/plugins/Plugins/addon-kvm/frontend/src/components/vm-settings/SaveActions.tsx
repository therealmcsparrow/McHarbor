// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { VM_PERM } from '../../permissions';
import type { VM } from '../../types';
import type { GroupName } from './settingsDraft';
import type { VmSettingsForm } from './useVmSettingsForm';

export function SaveActions({ vm, form, group }: { vm: VM; form: VmSettingsForm; group: GroupName }) {
  const { t } = useTranslation('vm');
  const dirty = Object.keys(form.changedIn(group)).length > 0;
  const locked = form.locked(group);

  return (
    <>
      {dirty && !locked && vm.state === 'stopped' && (
        <span className="mr-auto text-xs text-muted-foreground">{t('settings.appliesAtBoot')}</span>
      )}
      <Button
        permission={VM_PERM.MANAGE}
        onClick={() => form.save(group)}
        disabled={!dirty || locked || form.pending || (group === 'identity' && form.nameErrorKey !== null)}
      >
        {t('actions.save', { ns: 'common' })}
      </Button>
    </>
  );
}
