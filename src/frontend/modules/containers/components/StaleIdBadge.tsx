// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLink } from '@tabler/icons-react';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';

// StaleIdBadge marks a backup row whose container id was re-linked after the
// original container was recreated. Lives here rather than in
// backups-page-helpers so that file can stay a plain .ts module of pure
// helpers.
export function StaleIdBadge() {
  const { t } = useTranslation('containers');
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="inline-flex items-center gap-1 bg-warning/10 px-1.5 py-0.5 text-[10px] font-medium tracking-wide text-warning uppercase"
          data-testid="stale-id-badge"
        >
          <IconLink className="size-3" aria-hidden="true" />
          {t('backups.staleIdRelinked')}
        </span>
      </TooltipTrigger>
      <TooltipContent>{t('backups.staleIdTooltip')}</TooltipContent>
    </Tooltip>
  );
}
