// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// StackDetailHeader — the hero, built to `ContainerDetailHeader`'s grammar.
//
// This used to portal into the topbar: the global chrome carried a dozen
// stack-specific buttons. The identity — tile, name, chips — goes through
// `PageHeadIdentity` into the shell's page-head band, which is per-page
// rather than global, and the actions follow it through `PageHeadActions`,
// right-aligned on the same line. The topbar keeps the breadcrumb.
//
// The back arrow went with the move: its target was the first crumb's target.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconX, IconLock, IconStack2 } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { StackHeaderActions } from '../components/StackHeaderActions';
import type { StatusTone } from '@resources/theme/status-palette';
import { PERM } from '@core/auth/permissionKeys';

/**
 * A stack's status is its services' statuses collapsed into one word.
 * `partial` is the interesting one — some services up, some down — and it
 * gets warning rather than danger, because the stack is degraded, not dead.
 */
const STATUS_TONES: Record<string, StatusTone> = {
  running: 'success',
  partial: 'warning',
  stopped: 'danger',
};

type StackDetailHeaderProps = {
  stackName: string;
  status: string;
  isManaged: boolean;
  isRunning: boolean;
  serviceCount: number;
  editing?: boolean;
  onAction: (action: string) => void;
  onRemove: () => void;
  onTakeOver: () => void;
  onLinkContainer: () => void;
  onEdit?: () => void;
  onSave?: () => void;
  onCancelEdit?: () => void;
  saving?: boolean;
  locked?: boolean;
  targetsSelf?: boolean;
};

export function StackDetailHeader({
  stackName,
  status: stackStatus,
  isManaged,
  isRunning,
  serviceCount,
  editing,
  onAction,
  onRemove,
  onTakeOver,
  onLinkContainer,
  onEdit,
  onSave,
  onCancelEdit,
  saving,
  locked = false,
  targetsSelf = false,
}: StackDetailHeaderProps) {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const lockedLabel = tc('actions.locked');

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconStack2 className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {stackName}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <StatusIndicator
              tone={STATUS_TONES[stackStatus] ?? 'neutral'}
              label={stackStatus}
              size="sm"
              pulse={stackStatus === 'running'}
            />
            {locked && (
              <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
                <IconLock className="size-3" />
                {lockedLabel}
              </Badge>
            )}
            <Badge variant={isManaged ? 'default' : 'outline'} className="px-1.5 py-0 text-[10px]">
              {isManaged ? t('badges.managed') : t('badges.discovered')}
            </Badge>
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {t('detail.servicesCount')}: {serviceCount}
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
                tooltip={t('editStack.saveStack')}
                permission={PERM.STACKS_MANAGE}
                onClick={() => onSave?.()}
                variant="default"
                icon={
                  saving ? (
                    <span className="size-3.5 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                  ) : (
                    <IconCheck className="size-3.5" />
                  )
                }
              />
              <HeaderActionButton
                tooltip={t('editStack.cancel')}
                onClick={() => onCancelEdit?.()}
                icon={<IconX className="size-3.5" />}
                className="text-muted-foreground"
              />
            </>
          ) : (
            <StackHeaderActions
              isManaged={isManaged}
              isRunning={isRunning}
              locked={locked}
              onAction={onAction}
              onRemove={onRemove}
              onTakeOver={onTakeOver}
              onLinkContainer={onLinkContainer}
              onEdit={onEdit}
              targetsSelf={targetsSelf}
            />
          )}
        </div>
      </PageHeadActions>
    </>
  );
}
