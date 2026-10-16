// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconHeartFilled,
  IconArrowsTransferUp,
  IconCircleArrowUp,
  IconCircleCheck,
  IconAlertCircle,
  IconLock,
} from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import { isProtectedContainer } from '@core/utils/protection';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { ContainerIcon } from '../components/ContainerIcon';
import { HEALTH_COLORS, parseHealth, getStackName } from '../components/container-utils';

export type ContainerUpdateResult = { containerId: string; updateAvailable: boolean; error?: string };

export function ContainerNameCell({ container }: { container: ContainerInfo }) {
  const { t } = useTranslation('containers');
  return (
    <div className="flex min-w-0 items-center gap-x-2">
      <ContainerIcon image={container.Image} />
      <span className="truncate font-medium">{container.Names?.[0]?.replace(/^\//, '') ?? '-'}</span>
      {isProtectedContainer(container) && (
        <Tooltip>
          <TooltipTrigger asChild>
            <IconLock className="size-3.5 shrink-0 text-muted-foreground" />
          </TooltipTrigger>
          <TooltipContent>{t('actions.locked', { ns: 'common' })}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

export function ContainerHealthCell({ container }: { container: ContainerInfo }) {
  const { t } = useTranslation('containers');
  const health = parseHealth(container.Status);
  const color = health ? (HEALTH_COLORS[health] ?? 'text-muted-foreground') : 'text-muted-foreground/40';
  const label = health ? t(`health.${health}`) : t('health.noHealthcheck');
  return (
    <div className="flex justify-center">
      <Tooltip>
        <TooltipTrigger asChild>
          <IconHeartFilled className={`h-4 w-4 ${color}`} />
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function ContainerStackCell({
  container,
  onTakeOver,
}: {
  container: ContainerInfo;
  onTakeOver: (c: ContainerInfo) => void;
}) {
  const { t } = useTranslation('containers');
  const stack = getStackName(container);
  return stack ? (
    <Badge variant="default" className="max-w-full truncate px-1.5 py-0.5 text-[10px]">
      {stack}
    </Badge>
  ) : (
    <div className="flex items-center gap-1">
      <Badge variant="secondary" className="px-1.5 py-0.5 text-[10px]">
        {t('badges.standalone')}
      </Badge>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className="size-5"
            aria-label={t('actions.takeOver')}
            onClick={(clickEvent) => {
              clickEvent.stopPropagation();
              onTakeOver(container);
            }}
          >
            <IconArrowsTransferUp className="size-3 text-chart-3" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('actions.takeOver')}</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function ContainerUpdateCell({ result }: { result: ContainerUpdateResult | undefined }) {
  const { t } = useTranslation('containers');
  if (!result) return <span className="text-xs text-muted-foreground">-</span>;
  if (result.error) {
    return (
      <div className="flex justify-center">
        <Tooltip>
          <TooltipTrigger asChild>
            <IconAlertCircle className="size-4 text-warning" />
          </TooltipTrigger>
          <TooltipContent>{t('updates.checkFailed')}</TooltipContent>
        </Tooltip>
      </div>
    );
  }
  return (
    <div className="flex justify-center">
      <Tooltip>
        <TooltipTrigger asChild>
          {result.updateAvailable ? (
            <IconCircleArrowUp className="size-4 text-success" />
          ) : (
            <IconCircleCheck className="size-4 text-muted-foreground/50" />
          )}
        </TooltipTrigger>
        <TooltipContent>{result.updateAvailable ? t('updates.available') : t('updates.upToDate')}</TooltipContent>
      </Tooltip>
    </div>
  );
}
