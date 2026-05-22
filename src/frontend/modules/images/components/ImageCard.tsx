// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import type { ImageInfo } from '@core/types/docker';
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

  return (
    <Card
      className="cursor-pointer transition-colors hover:border-primary/40"
      onClick={() => onClick(image)}
    >
      <CardContent className="flex-1 space-y-3 p-4">
        <div className="flex items-start justify-between gap-2">
          <span className="min-w-0 truncate font-medium text-sm">{tag}</span>
          {image.Containers > 0 ? (
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
            {image.RepoTags.slice(1, 4).map((tag) => (
              <Badge key={tag} variant="secondary" className="text-[9px] px-1.5 py-0 truncate max-w-[140px]">
                {tag}
              </Badge>
            ))}
            {image.RepoTags.length > 4 && (
              <Badge variant="secondary" className="text-[9px] px-1.5 py-0">
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
              aria-label={tc('actions.remove')}
              onClick={() => onRemove(image.Id)}
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
