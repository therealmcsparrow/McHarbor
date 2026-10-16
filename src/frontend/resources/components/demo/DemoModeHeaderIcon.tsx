// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconFlask2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { useDemoStatus } from '@resources/hooks/useDemoMode';
import { demoVisitKey, useDemoBannerStore } from '@resources/stores/demoBannerStore';

export function DemoModeHeaderIcon() {
  const { t } = useTranslation('demo');
  const currentStatus = useDemoStatus();
  const dismissedFor = useDemoBannerStore((s) => s.dismissedFor);
  const show = useDemoBannerStore((s) => s.show);
  if (!currentStatus.data?.active || dismissedFor !== demoVisitKey(currentStatus.data.enteredAt)) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t('banner.show')} onClick={show}>
          <IconFlask2 className="size-4 text-warning" aria-hidden="true" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t('banner.show')}</TooltipContent>
    </Tooltip>
  );
}
