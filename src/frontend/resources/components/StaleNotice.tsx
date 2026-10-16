// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { formatDateCustom } from '@resources/utils/format';
import { useOnline } from '@resources/hooks/useOnline';
import { IconCloudOff } from '@tabler/icons-react';

export function StaleNotice({ cachedAt }: { cachedAt: string | null }) {
  const { t } = useTranslation('oncall');
  const online = useOnline();
  if (!cachedAt) return null;
  const time = formatDateCustom(cachedAt, { dateStyle: 'short', timeStyle: 'short' });
  return (
    <output className="mb-3 flex items-start gap-2 rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-foreground">
      <IconCloudOff className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span>{online ? t('stale.cached', { time }) : t('stale.offline', { time })}</span>
    </output>
  );
}
