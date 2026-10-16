// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconDeviceDesktop, IconLogout } from '@tabler/icons-react';
import { useAuth } from '@core/auth/useAuth';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { useLayoutModeStore, type LayoutMode } from '@resources/stores/layoutMode';
import { DeviceList } from '../components/DeviceList';
import { PushCard } from '../components/PushCard';

export default function DevicePage() {
  const { t } = useTranslation('oncall');
  const mode = useLayoutModeStore((s) => s.mode);
  const setMode = useLayoutModeStore((s) => s.setMode);
  const logout = useAuth((s) => s.logout);

  return (
    <section aria-labelledby="oncall-device-title" className="flex flex-col gap-3">
      <h1 id="oncall-device-title" className="text-lg font-semibold">
        {t('device.title')}
      </h1>
      <PushCard />
      <DeviceList />
      <div className="flex flex-col gap-2 rounded-lg border border-border bg-card p-4">
        <h2 className="font-medium">{t('device.layoutTitle')}</h2>
        <Select
          value={mode}
          onChange={(value) => setMode(value as LayoutMode)}
          ariaLabel={t('device.layoutTitle')}
          options={[
            { value: 'auto', label: t('device.layoutAuto') },
            { value: 'oncall', label: t('device.layoutOncall') },
            { value: 'desktop', label: t('device.layoutDesktop') },
          ]}
        />
        <Button asChild variant="outline" className="min-h-12">
          <Link to="/dashboard" onClick={() => setMode('desktop')}>
            <IconDeviceDesktop className="size-5" aria-hidden="true" />
            {t('device.openDesktop')}
          </Link>
        </Button>
      </div>
      <Button variant="ghost" className="min-h-12" onClick={logout}>
        <IconLogout className="size-5" aria-hidden="true" />
        {t('device.signOut')}
      </Button>
    </section>
  );
}
