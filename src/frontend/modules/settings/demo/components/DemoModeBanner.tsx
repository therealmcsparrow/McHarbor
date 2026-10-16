// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconFlask2, IconX } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Flex } from '@resources/components/ui/Layout';
import { useDemoStatus } from '@resources/hooks/useDemoMode';
import { demoVisitKey, useDemoBannerStore } from '@resources/stores/demoBannerStore';
import { DemoActionControl } from './DemoActionControl';

export function DemoModeBanner({ className }: { className?: string }) {
  const { t } = useTranslation('demo');
  const currentStatus = useDemoStatus();
  const visit = demoVisitKey(currentStatus.data?.enteredAt);
  const dismissed = useDemoBannerStore((s) => s.dismissedFor === visit);
  const dismiss = useDemoBannerStore((s) => s.dismiss);
  if (!currentStatus.data?.active || dismissed) return null;

  return (
    <Alert
      variant="warning"
      className={className}
      icon={<IconFlask2 className="size-4 shrink-0" aria-hidden="true" />}
      title={t('banner.title')}
      description={t('banner.description')}
      action={
        <Flex align="center" gap={2} wrap>
          <DemoActionControl action="exit" active size="sm" variant="outline" />
          <DemoActionControl action="reset" active size="sm" variant="destructive" compact />
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('banner.dismiss')}
            title={t('banner.dismiss')}
            onClick={() => dismiss(visit)}
          >
            <IconX className="size-4" aria-hidden="true" />
          </Button>
        </Flex>
      }
    />
  );
}
