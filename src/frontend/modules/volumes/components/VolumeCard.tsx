// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLock, IconTrash } from '@tabler/icons-react';
import type { VolumeInfo } from '@core/types/docker';
import { isProtectedVolume } from '@core/utils/protection';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { formatDate } from '@resources/utils/format';

type VolumeCardProps = {
  volume: VolumeInfo;
  onRemove: (name: string) => void;
};

export function VolumeCard({ volume, onRemove }: VolumeCardProps) {
  const { t } = useTranslation('volumes');
  const { t: tc } = useTranslation('common');
  const locked = isProtectedVolume(volume);

  return (
    <Card className="transition-colors hover:border-primary/40">
      <CardContent className="flex-1 space-y-3 p-4">
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <span className="min-w-0 truncate font-medium text-sm">{volume.Name}</span>
            {locked && (
              <Badge variant="secondary" className="shrink-0 gap-1 text-[9px] px-1.5 py-0">
                <IconLock className="size-3" />
                {tc('actions.locked')}
              </Badge>
            )}
          </div>
          {volume.RefCount > 0 ? (
            <Badge variant="success" className="shrink-0 text-[10px] px-2 py-0.5">
              {t('badges.inUse')}
            </Badge>
          ) : (
            <Badge variant="warning" className="shrink-0 text-[10px] px-2 py-0.5">
              {t('badges.unused')}
            </Badge>
          )}
        </div>

        <div className="space-y-1 text-xs text-muted-foreground">
          <div className="flex items-center justify-between">
            <span>{t('columns.driver')}</span>
            <Badge variant="default" className="text-[9px] px-1.5 py-0">{volume.Driver}</Badge>
          </div>
          <div className="flex items-center justify-between">
            <span>{t('columns.created')}</span>
            <span>{formatDate(volume.CreatedAt)}</span>
          </div>
        </div>

        <p className="truncate text-xs font-mono text-muted-foreground">{volume.Mountpoint}</p>
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2" onClick={(e) => e.stopPropagation()}>
        <div className="flex-1" />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={tc('actions.remove')}
              disabled={locked}
              onClick={() => onRemove(volume.Name)}
            >
              <IconTrash className="h-3.5 w-3.5 text-destructive" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{tc('actions.remove')}</TooltipContent>
        </Tooltip>
      </CardFooter>
    </Card>
  );
}
