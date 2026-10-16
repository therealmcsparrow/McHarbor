// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// GroupDetailHero — the identity half of the group detail card.
//
// Same shape as `UserDetailHero`: a tinted tile, an <h1> naming what you
// are looking at, a metadata row, and the destination's actions opposite.
// The tile carries initials for the same reason the user hero does — one
// identical group-glyph on every group would identify nothing.
//
// System groups get a muted tile and disabled edit/delete actions: they are
// managed by the host, and the controls stay visible so the state is
// legible rather than mysterious.

import { useTranslation } from 'react-i18next';
import { IconLock, IconPencil, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { cn } from '@resources/utils/cn';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import type { Group } from '../hooks/useGroups';

type GroupDetailHeroProps = {
  group: Group;
  onEdit: () => void;
  onDelete: () => void;
};

/** Up to two initials from the group name. */
function initialsFor(itemName: string): string {
  const source = itemName.trim();
  const parts = source.split(/\s+/).filter(Boolean);
  const letters = parts.length > 1 ? `${parts[0]![0] ?? ''}${parts[parts.length - 1]![0] ?? ''}` : source.slice(0, 2);
  return letters.toUpperCase() || '?';
}

export function GroupDetailHero({ group, onEdit, onDelete }: GroupDetailHeroProps) {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');

  return (
    <div className="flex flex-1 flex-col gap-2.5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span
            aria-hidden="true"
            className={cn(
              'grid size-11 shrink-0 place-items-center rounded-xl text-sm font-semibold ring-1 ring-inset',
              group.isSystem
                ? 'bg-muted text-muted-foreground ring-border'
                : 'bg-primary/12 text-primary ring-primary/25',
            )}
          >
            {initialsFor(group.name)}
          </span>
          <div className="min-w-0">
            <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
              {group.name}
            </h1>
            <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1">
              {group.isSystem ? (
                <Badge variant="secondary">
                  <IconLock className="mr-1 size-3" aria-hidden="true" />
                  {t('groups.system')}
                </Badge>
              ) : (
                <Badge variant="outline">{t('groups.custom')}</Badge>
              )}
              <Badge variant="outline" className="tabular-nums">
                {t('groups.memberCount', { count: group.memberCount })}
              </Badge>
              {group.description && <span className="truncate text-xs text-muted-foreground">{group.description}</span>}
              <span className="text-xs text-muted-foreground">
                {t('groups.createdAt')}: {timeAgo(group.createdAt)}
              </span>
            </div>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-1.5">
          <HeaderActionButton
            tooltip={tc('actions.edit')}
            icon={<IconPencil className="size-4" />}
            onClick={onEdit}
            disabled={group.isSystem}
            permission={PERM.GROUPS_MANAGE}
          />
          <HeaderActionButton
            tooltip={t('groups.deleteGroup')}
            icon={<IconTrash className="size-4 text-destructive" />}
            onClick={onDelete}
            disabled={group.isSystem}
            permission={PERM.GROUPS_MANAGE}
          />
        </div>
      </div>
    </div>
  );
}
