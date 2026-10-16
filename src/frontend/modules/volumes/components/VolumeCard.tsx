// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLock, IconTrash } from '@tabler/icons-react';
import type { VolumeInfo } from '@core/types/docker';
import { isProtectedVolume } from '@core/utils/protection';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { formatDate } from '@resources/utils/format';

type VolumeCardProps = {
  volume: VolumeInfo;
  onRemove: (name: string) => void;
  onOpen: (name: string) => void;
};

export function VolumeCard({ volume, onRemove, onOpen }: VolumeCardProps) {
  const { t } = useTranslation('volumes');
  const { t: tc } = useTranslation('common');
  const lockedLabel = tc('actions.locked');
  const removeLabel = tc('actions.remove');
  const locked = isProtectedVolume(volume);

  return (
    <Card className="transition-colors hover:border-primary/40">
      {/* The name button stretches over the whole body (after:inset-0) so the
          card opens the volume while the footer's destructive button stays separate. */}
      <CardContent className="relative flex-1 space-y-3 p-4">
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <PlainButton
              onClick={() => onOpen(volume.Name)}
              className="min-w-0 cursor-pointer truncate text-left text-sm font-medium after:absolute after:inset-0 after:content-['']"
            >
              {volume.Name}
            </PlainButton>
            {locked && (
              <Badge variant="secondary" className="shrink-0 gap-1 px-1.5 py-0 text-[9px]">
                <IconLock className="size-3" />
                {lockedLabel}
              </Badge>
            )}
          </div>
          {volume.RefCount > 0 ? (
            <Badge variant="success" className="shrink-0 px-2 py-0.5 text-[10px]">
              {t('badges.inUse')}
            </Badge>
          ) : (
            <Badge variant="warning" className="shrink-0 px-2 py-0.5 text-[10px]">
              {t('badges.unused')}
            </Badge>
          )}
        </div>

        <div className="space-y-1 text-xs text-muted-foreground">
          <div className="flex items-center justify-between">
            <span>{t('columns.driver')}</span>
            <Badge variant="default" className="px-1.5 py-0 text-[9px]">
              {volume.Driver}
            </Badge>
          </div>
          <div className="flex items-center justify-between">
            <span>{t('columns.created')}</span>
            <span>{formatDate(volume.CreatedAt)}</span>
          </div>
        </div>

        <p className="truncate font-mono text-xs text-muted-foreground">{volume.Mountpoint}</p>
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2">
        <div className="flex-1" />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={removeLabel}
              disabled={locked}
              onClick={() => onRemove(volume.Name)}
            >
              <IconTrash className="h-3.5 w-3.5 text-destructive" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{removeLabel}</TooltipContent>
        </Tooltip>
      </CardFooter>
    </Card>
  );
}
