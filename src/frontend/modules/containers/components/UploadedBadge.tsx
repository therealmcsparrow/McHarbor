// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconUpload } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';

export function UploadedBadge({ runId }: { runId: string }) {
  const { t } = useTranslation('containers');
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="info" data-testid={`run-uploaded-${runId}`}>
          <IconUpload className="size-3" aria-hidden="true" />
          {t('backups.uploaded.label')}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>{t('backups.uploaded.tooltip')}</TooltipContent>
    </Tooltip>
  );
}
