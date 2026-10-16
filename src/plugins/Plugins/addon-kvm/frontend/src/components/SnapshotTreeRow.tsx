// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconHistory, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { VM_PERM } from '../permissions';
import { formatBytes, formatRelative } from '../format';
import type { Snapshot, VM } from '../types';

export function SnapshotRow({
  vm,
  snapshot,
  isCurrent,
  hiddenCount,
  onRevert,
  onDelete,
}: {
  vm: VM;
  snapshot: Snapshot;
  isCurrent: boolean;
  hiddenCount: number;
  onRevert: (snapshot: Snapshot) => void;
  onDelete: (snapshot: Snapshot) => void;
}) {
  const { t } = useTranslation('vm');
  const branched = snapshot.childCount > 1;

  const compactBadge = 'px-1.5 py-0 text-[10px]';
  const meta = `${formatRelative(snapshot.createdAt)}${
    snapshot.sizeBytes ? ` · ${formatBytes(snapshot.sizeBytes)}` : ''
  } · ${snapshot.mechanism}`;

  return (
    <div
      className={`flex min-w-0 flex-1 items-center gap-2 rounded-lg border bg-card px-3 py-1.5 ${
        isCurrent ? 'border-primary' : 'border-border'
      }`}
    >
      <span className="truncate text-sm font-medium text-foreground" title={snapshot.name}>
        {snapshot.name}
      </span>
      <ConsistencyBadge consistency={snapshot.consistency} />
      {snapshot.withMemory && (
        <Badge variant="secondary" className={compactBadge}>
          {t('snapshots.create.withMemory')}
        </Badge>
      )}
      {branched && (
        <Badge variant="outline" className={compactBadge}>
          {t('snapshots.tree.branches', { count: snapshot.childCount })}
        </Badge>
      )}
      {hiddenCount > 0 && (
        <Badge variant="secondary" className={compactBadge}>
          {t('snapshots.tree.hiddenCount', { count: hiddenCount })}
        </Badge>
      )}
      {snapshot.description && (
        <span className="hidden truncate text-xs text-muted-foreground lg:inline" title={snapshot.description}>
          {snapshot.description}
        </span>
      )}

      <span className="ml-auto hidden shrink-0 text-[11px] text-muted-foreground sm:inline">{meta}</span>

      <div className="flex shrink-0 gap-1">
        <RevertButton vm={vm} snapshot={snapshot} isCurrent={isCurrent} onRevert={onRevert} />
        <DeleteButton vm={vm} snapshot={snapshot} branched={branched} onDelete={onDelete} />
      </div>
    </div>
  );
}

function RevertButton({
  vm,
  snapshot,
  isCurrent,
  onRevert,
}: {
  vm: VM;
  snapshot: Snapshot;
  isCurrent: boolean;
  onRevert: (snapshot: Snapshot) => void;
}) {
  const { t } = useTranslation('vm');

  // A running guest is no longer a reason: the dialog shuts it down, with the
  // operator's consent, and starts it again afterwards if they asked.
  let reason = '';
  if (!vm.hostAvailable) reason = t('snapshots.tree.revertOfflineHint');
  else if (isCurrent) reason = t('snapshots.tree.revertCurrentHint');

  const button = (
    <Button
      variant="ghost"
      size="icon"
      aria-label={t('actions.revert')}
      className="size-6"
      permission={VM_PERM.MANAGE}
      disabled={reason !== ''}
      onClick={() => onRevert(snapshot)}
    >
      <IconHistory className="size-3.5" aria-hidden="true" />
    </Button>
  );
  if (reason === '') return button;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span>{button}</span>
      </TooltipTrigger>
      <TooltipContent>{reason}</TooltipContent>
    </Tooltip>
  );
}

function DeleteButton({
  vm,
  snapshot,
  branched,
  onDelete,
}: {
  vm: VM;
  snapshot: Snapshot;
  branched: boolean;
  onDelete: (snapshot: Snapshot) => void;
}) {
  const { t } = useTranslation('vm');
  const button = (
    <Button
      variant="ghost"
      size="icon"
      aria-label={t('actions.delete')}
      className="size-6"
      permission={VM_PERM.DELETE}
      disabled={!vm.hostAvailable || branched}
      onClick={() => onDelete(snapshot)}
    >
      <IconTrash className="size-3.5" aria-hidden="true" />
    </Button>
  );
  if (!branched) return button;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span>{button}</span>
      </TooltipTrigger>
      <TooltipContent>{t('snapshots.tree.branchedHint')}</TooltipContent>
    </Tooltip>
  );
}

function ConsistencyBadge({ consistency }: { consistency: Snapshot['consistency'] }) {
  const { t } = useTranslation('vm');
  const label = t(`snapshots.consistency.${consistency}`);
  const hint = t(`snapshots.consistency.${consistency}Hint`);

  const badge = (
    <Badge variant={consistency === 'crash' ? 'warning' : 'success'} className="px-1.5 py-0 text-[10px]">
      {label}
    </Badge>
  );

  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  );
}
