// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// VolumeDetailHeader — the hero, built to `ContainerDetailHeader`'s grammar.
//
// Same tile, same `font-display text-2xl` name, same chip row — all three in
// the page-head band via `PageHeadIdentity` — and the same action row,
// right-aligned in that band through `PageHeadActions`. A volume has no
// lifecycle, but it does have the fact you came here for: is anything
// mounting it, and how big has it got.

import { useTranslation } from 'react-i18next';
import { IconDatabase, IconLock, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { formatBytes } from '@resources/utils/format';
import type { VolumeInfo } from '@core/types/docker';
import { PERM } from '@core/auth/permissionKeys';

type VolumeDetailHeaderProps = {
  volume: VolumeInfo;
  /** Containers currently mounting it — drives the in-use / unused dot. */
  mountedCount: number;
  onRemove: () => void;
};

export function VolumeDetailHeader({ volume, mountedCount, onRemove }: VolumeDetailHeaderProps) {
  const { t } = useTranslation('volumes');
  const { t: tc } = useTranslation('common');
  const lockedLabel = tc('actions.locked');
  const inUse = mountedCount > 0;
  const locked = Boolean(volume.Protected);
  const size = volume.UsageData?.Size;

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconDatabase className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {volume.Name}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <StatusIndicator
              tone={inUse ? 'success' : 'neutral'}
              label={inUse ? t('badges.inUse') : t('badges.unused')}
              size="sm"
            />
            {locked && (
              <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
                <IconLock className="size-3" />
                {lockedLabel}
              </Badge>
            )}
            <Badge variant="default" className="px-1.5 py-0 text-[10px]">
              {volume.Driver}
            </Badge>
            {volume.Scope && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                {volume.Scope}
              </Badge>
            )}
            {/* Size is only present when the daemon was asked for usage
                  data; a volume without it says nothing rather than "0 B". */}
            {size !== undefined && (
              <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                {formatBytes(size)}
              </span>
            )}
            <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {volume.Mountpoint}
            </span>
          </div>
        </div>
      </PageHeadIdentity>

      {/* Actions, right-aligned in the page-head band beside the identity.
          Removing a volume that a container still mounts fails at the daemon,
          so the button says why before you click it. */}
      <PageHeadActions>
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          <HeaderActionButton
            tooltip={
              locked ? t('detail.protectedCannotRemove') : inUse ? t('detail.inUseCannotRemove') : tc('actions.remove')
            }
            permission={PERM.VOLUMES_DELETE}
            onClick={onRemove}
            variant="destructive"
            disabled={locked || inUse}
            icon={<IconTrash className="size-3.5" />}
          />
        </div>
      </PageHeadActions>
    </>
  );
}
