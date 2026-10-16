// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ImageDetailHeader — the hero, built to `ContainerDetailHeader`'s grammar.
//
// This used to portal into the topbar, leaving the page itself with no title.
// The identity — tile, name, chips — goes through `PageHeadIdentity` into the
// shell's page-head band, and the actions follow it through `PageHeadActions`,
// right-aligned. The topbar keeps the breadcrumb, and the back arrow went with
// it — its target was the first crumb's target.

import { useTranslation } from 'react-i18next';
import { IconFileExport, IconLock, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { ContainerIcon } from '@resources/components/ContainerIcon';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { formatBytes, truncateId } from '@resources/utils/format';
import type { ImageInspect } from '@core/types/docker';
import { isProtectedImage } from '@core/utils/protection';
import { PERM } from '@core/auth/permissionKeys';

type ImageDetailHeaderProps = {
  image: ImageInspect;
  onExport: () => void;
  onRemove: () => void;
};

export function ImageDetailHeader({ image, onExport, onRemove }: ImageDetailHeaderProps) {
  const { t } = useTranslation('images');
  const { t: tc } = useTranslation('common');
  const shortId = truncateId(image.Id.replace('sha256:', ''));
  const primaryTag = image.RepoTags?.[0] ?? null;
  const displayName = primaryTag ?? shortId;
  const locked = isProtectedImage(image);
  const lockedLabel = tc('actions.locked');
  // An image with no repo tag is dangling — the thing you prune. That is the
  // one piece of state an image has, so it takes the dot the container hero
  // spends on running/stopped.
  const dangling = !primaryTag || primaryTag === '<none>:<none>';
  const extraTags = (image.RepoTags?.length ?? 0) - 1;

  return (
    <>
      <PageHeadIdentity>
        {/* The image's own mark, exactly as the container hero draws it —
              a homelab is a wall of familiar logos, and the eye lands on the
              logo long before it parses the tag. */}
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <ContainerIcon image={primaryTag ?? ''} className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {displayName}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <StatusIndicator
              tone={dangling ? 'neutral' : 'success'}
              label={dangling ? t('detail.badges.dangling') : t('detail.badges.tagged')}
              size="sm"
            />
            {locked && (
              <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
                <IconLock className="size-3" />
                {lockedLabel}
              </Badge>
            )}
            {extraTags > 0 && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                +{extraTags}
              </Badge>
            )}
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {formatBytes(image.Size)}
            </span>
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {shortId}
            </span>
          </div>
        </div>
      </PageHeadIdentity>

      {/* Actions, right-aligned in the same page-head band as the identity
          they act on. */}
      <PageHeadActions>
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          <HeaderActionButton
            tooltip={t('export.title')}
            permission={PERM.IMAGES_MANAGE}
            onClick={onExport}
            icon={<IconFileExport className="size-3.5" />}
          />
          <div className="mx-0.5 h-5 w-px bg-border" />
          <HeaderActionButton
            tooltip={tc('actions.remove')}
            permission={PERM.IMAGES_DELETE}
            onClick={onRemove}
            variant="destructive"
            disabled={locked}
            icon={<IconTrash className="size-3.5" />}
          />
        </div>
      </PageHeadActions>
    </>
  );
}
