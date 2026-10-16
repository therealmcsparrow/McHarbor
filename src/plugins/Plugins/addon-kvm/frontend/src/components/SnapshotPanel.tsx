// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCameraPlus, IconHistory } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { StatusError } from '@resources/components/ui/StatusError';
import { ProgressLine } from './ProgressLine';
import { SnapshotTree } from './SnapshotTree';
import { VM_PERM } from '../permissions';
import {
  useCreateSnapshot,
  useDeleteSnapshot,
  useRevertSnapshot,
  useVMProgress,
  useVMSnapshots,
} from '../hooks/useVMs';
import type { Snapshot, VM } from '../types';
import { CreateSnapshotDialog } from './CreateSnapshotDialog';
import { RevertDialog } from './RevertSnapshotDialog';

/** Chain limits, mirrored from the backend so the UI warns before it refuses. */
const MAX_CHAIN = 16;
const WARN_CHAIN = 8;

/**
 * A guest's snapshots.
 *
 * The panel is opinionated about two things the operator would otherwise learn
 * the hard way:
 *
 *   - What a snapshot would actually restore as. Without the guest agent it is
 *     crash-consistent — the equivalent of pulling the power — and saying so is
 *     the difference between a restore that works and one that surprises.
 *   - That a snapshot is not a backup. It lives on the same storage as the
 *     guest, so a pool failure takes both.
 */
export function SnapshotPanel({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const { data, isLoading } = useVMSnapshots(vm.id);
  const revert = useRevertSnapshot();
  const remove = useDeleteSnapshot();

  const [creating, setCreating] = useState(false);
  // The snapshot being taken. The dialog closes the moment it is submitted —
  // a snapshot with memory runs for minutes, and a modal is the wrong place to
  // wait it out — so the panel carries the pending one and draws it in the
  // tree with the progress the host reports.
  const [pending, setPending] = useState<{ name: string; withMemory: boolean } | null>(null);
  const [failed, setFailed] = useState<{ name: string; error: string } | null>(null);
  const create = useCreateSnapshot();
  const [reverting_, setReverting_] = useState<string | null>(null);
  const progress = useVMProgress(vm.id, pending !== null || reverting_ !== null);
  const revertStep = progress.data?.revertProgress;

  // The guest stops reporting once the revert is done, which is the signal to
  // stop polling and let the ten-second refetch take over again. Held for a
  // moment first: a revert of a stopped guest is over in about a second, and a
  // row that appears and vanishes between two frames reads as nothing having
  // happened at all.
  useEffect(() => {
    if (!reverting_ || !progress.data || progress.data.revertProgress) return;
    const timer = window.setTimeout(() => setReverting_(null), 1500);
    return () => window.clearTimeout(timer);
  }, [reverting_, progress.data]);
  const [reverting, setReverting] = useState<Snapshot | null>(null);
  const [restartAfter, setRestartAfter] = useState(true);
  const [deleting, setDeleting] = useState<Snapshot | null>(null);

  const snapshots = data ?? [];
  const depth = snapshots.reduce((deepest, entry) => Math.max(deepest, entry.chainDepth), 0);

  if (isLoading) return <Spinner className="h-6 w-6" />;

  return (
    <div className="space-y-4">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t('snapshots.notABackup')}</p>
        <Button
          size="sm"
          permission={VM_PERM.MANAGE}
          disabled={!vm.hostAvailable || depth >= MAX_CHAIN}
          onClick={() => setCreating(true)}
        >
          <IconCameraPlus className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {t('actions.snapshot')}
        </Button>
      </header>

      {depth >= MAX_CHAIN ? (
        <p className="rounded-md bg-destructive/10 p-3 text-sm text-foreground">{t('snapshots.chainFull')}</p>
      ) : depth >= WARN_CHAIN ? (
        <p className="rounded-md bg-warning/10 p-3 text-sm text-foreground">{t('snapshots.chainWarning', { depth })}</p>
      ) : null}

      {pending && (
        <ProgressLine
          title={pending.name}
          step={t(`snapshots.progress.${progress.data?.snapshotProgress?.step ?? 'starting'}`)}
          percent={progress.data?.snapshotProgress?.percent ?? 5}
        />
      )}

      {reverting_ && (
        <ProgressLine
          title={t('snapshots.revert.running')}
          step={t(`snapshots.revertStep.${revertStep?.step ?? 'starting'}`)}
          percent={revertStep?.percent ?? 5}
        />
      )}

      {failed && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/5 px-3 py-1.5">
          <span className="truncate text-sm text-foreground">
            {t('snapshots.progress.failed', { name: failed.name })}
          </span>
          <StatusError error={failed.error} className="ml-auto" />
          <Button variant="ghost" size="sm" onClick={() => setFailed(null)}>
            {t('actions.close', { ns: 'common' })}
          </Button>
        </div>
      )}

      {snapshots.length === 0 && !pending ? (
        <EmptyState
          icon={<IconHistory className="h-6 w-6" aria-hidden="true" />}
          title={t('snapshots.empty.title')}
          description={t('snapshots.empty.body')}
        />
      ) : (
        <SnapshotTree vm={vm} snapshots={snapshots} onRevert={setReverting} onDelete={setDeleting} />
      )}

      {creating && (
        <CreateSnapshotDialog
          vm={vm}
          onClose={() => setCreating(false)}
          onSubmit={(input) => {
            setCreating(false);
            setFailed(null);
            setPending({ name: input.name, withMemory: Boolean(input.withMemory) });
            create.mutate(
              { id: vm.id, ...input },
              {
                onError: (error: Error) => setFailed({ name: input.name, error: error.message }),
                onSettled: () => setPending(null),
              },
            );
          }}
        />
      )}

      {reverting && (
        <RevertDialog
          vm={vm}
          snapshot={reverting}
          pending={revert.isPending}
          restartAfter={restartAfter}
          onRestartAfterChange={setRestartAfter}
          onClose={() => setReverting(null)}
          onConfirm={() => {
            const snapshotId = reverting.id;
            setReverting(null);
            setReverting_(snapshotId);
            revert.mutate(
              {
                id: vm.id,
                snapshotId,
                stopFirst: vm.state === 'running',
                restartAfter: vm.state === 'running' && restartAfter,
              },
              { onError: (_error) => setReverting_(null) },
            );
          }}
        />
      )}

      {deleting && (
        <ConfirmDialog
          open
          onOpenChange={(isOpen) => !isOpen && setDeleting(null)}
          title={t('snapshots.delete.title', { name: deleting.name })}
          description={t('snapshots.delete.body')}
          confirmLabel={t('snapshots.delete.submit')}
          variant="destructive"
          loading={remove.isPending}
          onConfirm={() =>
            remove.mutate({ id: vm.id, snapshotId: deleting.id }, { onSuccess: () => setDeleting(null) })
          }
        />
      )}
    </div>
  );
}
