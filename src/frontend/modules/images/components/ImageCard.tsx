// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLock, IconTrash } from '@tabler/icons-react';
import type { ImageInfo } from '@core/types/docker';
import { isProtectedImage } from '@core/utils/protection';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { formatBytes, timeAgo, truncateId } from '@resources/utils/format';

type ImageCardProps = {
  image: ImageInfo;
  onClick: (img: ImageInfo) => void;
  onRemove: (id: string) => void;
};

export function ImageCard({ image, onClick, onRemove }: ImageCardProps) {
  const { t } = useTranslation('images');
  const { t: tc } = useTranslation('common');
  const tag = image.RepoTags?.[0] ?? '<none>';
  const shortId = truncateId((image.Id ?? '').replace('sha256:', ''));
  const locked = isProtectedImage(image);
  const lockedLabel = tc('actions.locked');
  const removeLabel = tc('actions.remove');

  return (
    <Card className="cursor-pointer transition-colors hover:border-primary/40" onClick={() => onClick(image)}>
      <CardContent className="flex-1 space-y-3 p-4">
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <span className="min-w-0 truncate text-sm font-medium">{tag}</span>
            {locked && (
              <Badge variant="secondary" className="shrink-0 gap-1 px-1.5 py-0 text-[9px]">
                <IconLock className="size-3" />
                {lockedLabel}
              </Badge>
            )}
          </div>
          {image.Containers > 0 ? (
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
            <span>{t('columns.id')}</span>
            <span className="font-mono">{shortId}</span>
          </div>
          <div className="flex items-center justify-between">
            <span>{t('columns.size')}</span>
            <span>{formatBytes(image.Size)}</span>
          </div>
          <div className="flex items-center justify-between">
            <span>{t('columns.created')}</span>
            <span>{timeAgo(new Date(image.Created * 1000).toISOString())}</span>
          </div>
        </div>

        {image.RepoTags && image.RepoTags.length > 1 && (
          <div className="flex flex-wrap gap-1">
            {image.RepoTags.slice(1, 4).map((repoTag) => (
              <Badge key={repoTag} variant="secondary" className="max-w-[140px] truncate px-1.5 py-0 text-[9px]">
                {repoTag}
              </Badge>
            ))}
            {image.RepoTags.length > 4 && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[9px]">
                +{image.RepoTags.length - 4}
              </Badge>
            )}
          </div>
        )}
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2" onClick={(e) => e.stopPropagation()}>
        <div className="flex-1" />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={removeLabel}
              disabled={locked}
              onClick={() => onRemove(image.Id)}
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
