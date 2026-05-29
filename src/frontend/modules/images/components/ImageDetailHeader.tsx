// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { IconArrowLeft, IconFileExport, IconLock, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { truncateId } from '@resources/utils/format';
import type { ImageInspect } from '@core/types/docker';
import { isProtectedImage } from '@core/utils/protection';
import { Badge } from '@resources/components/ui/Badge';

type ImageDetailHeaderProps = {
  image: ImageInspect;
  onBack: () => void;
  onExport: () => void;
  onRemove: () => void;
};

export function ImageDetailHeader({ image, onBack, onExport, onRemove }: ImageDetailHeaderProps) {
  const { t } = useTranslation('images');
  const { t: tc } = useTranslation('common');
  const setHeaderActive = useHeaderSlot((state) => state.setActive);
  const headerSlot = document.getElementById('header-slot');
  const name = image.RepoTags?.[0] ?? truncateId(image.Id.replace('sha256:', ''));
  const locked = isProtectedImage(image);

  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  if (!headerSlot) {
    return null;
  }

  return createPortal(
    <div className="flex flex-1 items-center justify-between">
      <div className="flex items-center gap-3">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={t('detail.back')}
              onClick={onBack}
            >
              <IconArrowLeft className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('detail.back')}</TooltipContent>
        </Tooltip>
        <div className="h-5 w-px bg-border" />
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-sm font-semibold text-foreground">{name}</h1>
            {locked && (
              <Badge variant="secondary" className="gap-1 text-[10px] px-1.5 py-0">
                <IconLock className="size-3" />
                {tc('actions.locked')}
              </Badge>
            )}
          </div>
          <p className="font-mono text-xs text-muted-foreground">
            {truncateId(image.Id.replace('sha256:', ''))}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-1.5">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={t('export.title')}
              onClick={onExport}
            >
              <IconFileExport className="size-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('export.title')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={tc('actions.remove')}
              onClick={onRemove}
              disabled={locked}
              className="text-red-500 hover:border-red-500/30 hover:bg-red-500/10"
            >
              <IconTrash className="size-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{tc('actions.remove')}</TooltipContent>
        </Tooltip>
      </div>
    </div>,
    headerSlot,
  );
}
