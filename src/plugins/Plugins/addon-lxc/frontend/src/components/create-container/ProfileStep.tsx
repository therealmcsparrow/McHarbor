// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconLock, IconLockOpen } from '@tabler/icons-react';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import type { CreateContainerForm } from './useCreateContainerForm';
import type { InitSystem } from '../../types';

export function ProfileStep({ form }: { form: CreateContainerForm }) {
  const { t } = useTranslation('ct');
  const { initSystem, setInitSystem, privileged, setPrivileged, nesting, setNesting } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.profile.title')}</h3>
      <div>
        <Label htmlFor="ct-init">{t('create.profile.init')}</Label>
        <Select
          value={initSystem}
          ariaLabel={t('create.profile.init')}
          options={[
            // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: init system name
            { value: 'systemd', label: 'systemd' },
            // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: init system name
            { value: 'openrc', label: 'OpenRC' },
            { value: 'none', label: t('state.defined') },
          ]}
          onChange={(value) => setInitSystem(value as InitSystem)}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('create.profile.initHint')}</p>
      </div>

      <div className="rounded-md border border-border p-3">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-2">
            {privileged ? (
              <IconLockOpen className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
            ) : (
              <IconLock className="mt-0.5 h-4 w-4 shrink-0 text-success" aria-hidden="true" />
            )}
            <div>
              <Label htmlFor="ct-privileged" className="text-sm font-normal">
                {privileged ? t('create.profile.privileged') : t('create.profile.unprivileged')}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {privileged ? t('create.profile.privilegedHint') : t('create.profile.unprivilegedHint')}
              </p>
            </div>
          </div>
          <Switch id="ct-privileged" checked={privileged} onCheckedChange={setPrivileged} />
        </div>

        {privileged && (
          <div className="mt-3 flex items-start gap-2 rounded-md bg-destructive/10 p-3">
            <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
            <p className="text-sm text-foreground">
              {t('create.profile.privilegedHint')} — {t('create.profile.privilegeRequired')}
            </p>
          </div>
        )}
      </div>

      <div className="flex items-start justify-between gap-3">
        <div>
          <Label htmlFor="ct-nesting" className="text-sm font-normal">
            {t('create.profile.nesting')}
          </Label>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('create.profile.nestingHint')}</p>
        </div>
        <Switch id="ct-nesting" checked={nesting} onCheckedChange={setNesting} />
      </div>
    </>
  );
}
