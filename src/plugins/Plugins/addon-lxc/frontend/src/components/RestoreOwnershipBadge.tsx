// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import type { BackupRun } from '../hooks/useCTBackups';

export function RestoreOwnershipBadge({ run }: { run: BackupRun }) {
  const { t } = useTranslation('ct');
  if (run.operation !== 'restore') return null;

  let label: string;
  let hint: string;
  let variant: 'secondary' | 'info' | 'warning';
  switch (run.idShift) {
    case 'none':
      label = t('backups.idShift.none');
      hint = t('backups.idShift.noneHint');
      variant = 'secondary';
      break;
    case 'shifted':
      label = t('backups.idShift.shifted');
      hint = t('backups.idShift.shiftedHint');
      variant = 'info';
      break;
    case 'skipped':
      label = t('backups.idShift.skipped');
      hint = t('backups.idShift.skippedHint');
      variant = 'warning';
      break;
    default:
      return null;
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant={variant} tabIndex={0} aria-label={hint}>
          {label}
        </Badge>
      </TooltipTrigger>
      <TooltipContent className="max-w-[340px]">{hint}</TooltipContent>
    </Tooltip>
  );
}
