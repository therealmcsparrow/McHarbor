// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';

/** The agent's notice for a device the running guest never picked up. */
export const HOTPLUG_UNCLAIMED = 'hotplug_unclaimed';

/**
 * Says a live add worked on the host but not in the guest. Shown in the dialog
 * rather than as a toast, because the operator has to read it before closing.
 */
export function HotplugNotice({ notice }: { notice: string }) {
  const { t } = useTranslation('vm');
  if (notice !== HOTPLUG_UNCLAIMED) return null;
  return (
    <output className="flex items-start gap-2 text-sm text-warning">
      <IconAlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      {t('hardware.hotplugUnclaimed')}
    </output>
  );
}
