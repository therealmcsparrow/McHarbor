// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// NetworkDetailHeader — the hero, built to `ContainerDetailHeader`'s grammar.
//
// This used to portal its controls into the topbar, which is the arrangement
// the container page abandoned: the global chrome ended up carrying
// network-specific buttons. The identity — tile, name, chips — goes through
// `PageHeadIdentity` into the shell's page-head band, which is per-page
// rather than global, and the actions follow it through `PageHeadActions`,
// right-aligned. The topbar keeps only the breadcrumb.
//
// The back arrow went with the move: its target was the first crumb's target.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconLoader2, IconLock, IconNetwork, IconPencil, IconTrash, IconX } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { truncateId } from '@resources/utils/format';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { PERM } from '@core/auth/permissionKeys';

type NetworkDetailHeaderProps = {
  name: string;
  id: string;
  driver: string;
  scope: string;
  builtIn: boolean;
  /** Containers currently attached — drives the used / unused state dot. */
  connectedCount: number;
  editing: boolean;
  saving?: boolean;
  onEdit: () => void;
  onSave: () => void;
  onCancelEdit: () => void;
  onRemove: () => void;
};

export function NetworkDetailHeader({
  name: networkName,
  id,
  driver,
  scope,
  builtIn,
  connectedCount,
  editing,
  saving,
  onEdit,
  onSave,
  onCancelEdit,
  onRemove,
}: NetworkDetailHeaderProps) {
  const { t } = useTranslation('networks');
  const inUse = connectedCount > 0;

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconNetwork className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {networkName}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            {/* A network has no lifecycle to report, but it does have the
                  one fact you came here for: is anything actually on it. */}
            <StatusIndicator
              tone={inUse ? 'success' : 'neutral'}
              label={inUse ? t('badges.used') : t('badges.unused')}
              size="sm"
            />
            {builtIn && (
              <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
                <IconLock className="size-3" />
                {t('badges.builtIn')}
              </Badge>
            )}
            <Badge variant="default" className="px-1.5 py-0 text-[10px]">
              {driver}
            </Badge>
            <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
              {scope}
            </Badge>
            {/* The id as a mono chip on the state line — an identifier, not
                  prose, and scannable at a glance. */}
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {truncateId(id)}
            </span>
          </div>
        </div>
      </PageHeadIdentity>

      {/* Actions, right-aligned in the same page-head band as the identity
          they act on. */}
      <PageHeadActions>
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          {editing ? (
            <>
              <HeaderActionButton
                tooltip={t('edit.saveChanges')}
                permission={PERM.NETWORKS_MANAGE}
                onClick={onSave}
                icon={saving ? <IconLoader2 className="size-3.5 animate-spin" /> : <IconCheck className="size-3.5" />}
                className="text-success hover:border-success/30 hover:bg-success/10"
              />
              <HeaderActionButton
                tooltip={t('edit.cancelChanges')}
                onClick={onCancelEdit}
                icon={<IconX className="size-3.5" />}
                className="text-muted-foreground hover:border-border hover:bg-muted/50"
              />
            </>
          ) : (
            <>
              <HeaderActionButton
                tooltip={builtIn ? t('detail.builtInCannotEdit') : t('actions.edit')}
                permission={PERM.NETWORKS_MANAGE}
                onClick={onEdit}
                icon={<IconPencil className="size-3.5" />}
                className="text-info hover:border-info/30 hover:bg-info/10"
                disabled={builtIn}
              />
              <div className="mx-0.5 h-5 w-px bg-border" />
              <HeaderActionButton
                tooltip={builtIn ? t('detail.builtInCannotRemove') : t('actions.remove')}
                permission={PERM.NETWORKS_DELETE}
                onClick={onRemove}
                variant="destructive"
                icon={<IconTrash className="size-3.5" />}
                disabled={builtIn}
              />
            </>
          )}
        </div>
      </PageHeadActions>
    </>
  );
}
