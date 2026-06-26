// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash, IconPencil, IconLock } from '@tabler/icons-react';
import type { NetworkInfo } from '@core/types/docker';
import { getNetworkContainerCount, isBuiltInNetwork, isNetworkInUse } from '@core/utils/network';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { truncateId } from '@resources/utils/format';

type NetworkCardProps = {
  network: NetworkInfo;
  onClick: (net: NetworkInfo) => void;
  onRemove: (id: string) => void;
};

export function NetworkCard({ network, onClick, onRemove }: NetworkCardProps) {
  const { t } = useTranslation('networks');

  const containerCount = getNetworkContainerCount(network);
  const inUse = isNetworkInUse(network);
  const builtIn = isBuiltInNetwork(network);

  return (
    <Card
      className="cursor-pointer transition-colors hover:border-primary/40"
      onClick={() => onClick(network)}
    >
      <CardContent className="flex-1 space-y-3 p-4">
        <div className="flex items-start justify-between gap-2">
          <span className="flex min-w-0 items-center gap-1.5 truncate font-medium text-sm">
            {builtIn && <IconLock className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />}
            <span className="truncate">{network.Name}</span>
          </span>
          <div className="flex shrink-0 items-center gap-1">
            {builtIn && (
              <Badge variant="outline" className="text-[10px] px-2 py-0.5">
                {t('badges.builtIn')}
              </Badge>
            )}
            <Badge
              variant={inUse ? 'success' : 'secondary'}
              className="text-[10px] px-2 py-0.5"
            >
              {inUse ? t('badges.used') : t('badges.unused')}
            </Badge>
            <Badge variant="default" className="text-[10px] px-2 py-0.5">
              {network.Driver}
            </Badge>
          </div>
        </div>

        <div className="space-y-1 text-xs text-muted-foreground">
          <div className="flex items-center justify-between">
            <span>{t('columns.id')}</span>
            <span className="font-mono">{truncateId(network.Id)}</span>
          </div>
          <div className="flex items-center justify-between">
            <span>{t('columns.scope')}</span>
            <Badge variant="secondary" className="text-[9px] px-1.5 py-0">{network.Scope}</Badge>
          </div>
          <div className="flex items-center justify-between">
            <span>{t('columns.containers')}</span>
            <span className="tabular-nums text-foreground">{containerCount}</span>
          </div>
          {network.Internal && (
            <div className="flex items-center justify-between">
              <span>{t('columns.internal')}</span>
              <Badge variant="warning" className="text-[9px] px-1.5 py-0">{t('badges.yes')}</Badge>
            </div>
          )}
        </div>

        {network.IPAM?.Config?.[0]?.Subnet && (
          <Badge variant="outline" className="text-[9px] px-1.5 py-0 font-mono">
            {network.IPAM.Config[0].Subnet}
          </Badge>
        )}
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2" onClick={(e) => e.stopPropagation()}>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.edit')}
              onClick={() => onClick(network)}
            >
              <IconPencil className="h-3.5 w-3.5 text-primary" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('actions.edit')}</TooltipContent>
        </Tooltip>
        <div className="flex-1" />
        {builtIn ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <span tabIndex={0} className="inline-flex">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('actions.remove')}
                  disabled
                  className="pointer-events-none opacity-40"
                >
                  <IconTrash className="h-3.5 w-3.5 text-destructive" />
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>{t('toast.builtIn')}</TooltipContent>
          </Tooltip>
        ) : (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('actions.remove')}
                onClick={() => onRemove(network.Id)}
              >
                <IconTrash className="h-3.5 w-3.5 text-destructive" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('actions.remove')}</TooltipContent>
          </Tooltip>
        )}
      </CardFooter>
    </Card>
  );
}
