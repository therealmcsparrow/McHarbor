// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCameraPlus, IconRestore, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { CT_PERM } from '../permissions';
import { useCreateSnapshot, useDeleteSnapshot, useRevertSnapshot } from '../hooks/useContainers';
import { formatBytes, formatRelative } from '../format';
import { isLive, type Container, type Snapshot } from '../types';

/** How deep a chain gets before it is worth saying something. */
const WARN_AT_CHAIN = 8;

/**
 * A container's snapshots.
 *
 * Two things this panel refuses to be vague about:
 *
 *   - **Consistency.** A snapshot taken while the container ran unfrozen is
 *     crash-consistent — anything not yet written to disk is simply missing,
 *     the same state a power cut produces. That is shown per row, because six
 *     months later it is the difference between a restore that boots and one
 *     that runs fsck, and nothing else will remember.
 *   - **Mechanism.** On a directory pool there is no snapshot mechanism, so
 *     what is taken is an rsync copy. Correct, far slower, and using the space
 *     to match. Labelling it "snapshot" without saying so would set an
 *     expectation the pool cannot meet.
 */
export function SnapshotsPanel({ container }: { container: Container }) {
  const { t } = useTranslation('ct');
  const createSnapshot = useCreateSnapshot();
  const revertSnapshot = useRevertSnapshot();
  const deleteSnapshot = useDeleteSnapshot();

  const [snapshotName, setSnapshotName] = useState('');
  const [freeze, setFreeze] = useState(true);
  const [reverting, setReverting] = useState<Snapshot | null>(null);

  const snapshots = container.snapshots ?? [];
  const live = isLive(container);
  const deepest = snapshots.reduce((max, snap) => Math.max(max, snap.chainDepth), 0);

  return (
    <div className="space-y-6">
      <section className="rounded-lg border border-border p-4">
        <h3 className="mb-3 font-medium text-foreground">{t('actions.snapshot')}</h3>
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-[14rem] flex-1">
            <Label htmlFor="snapshotName">{t('snapshots.columns.name')}</Label>
            <Input id="snapshotName" value={snapshotName} onChange={(ev) => setSnapshotName(ev.target.value)} />
          </div>
          <Button
            permission={CT_PERM.MANAGE}
            disabled={!snapshotName || createSnapshot.isPending || !container.hostAvailable}
            onClick={() => {
              createSnapshot.mutate(
                { id: container.id, name: snapshotName, freeze },
                { onSuccess: () => setSnapshotName('') },
              );
            }}
          >
            <IconCameraPlus className="mr-1.5 h-4 w-4" aria-hidden="true" />
            {t('actions.snapshot')}
          </Button>
        </div>

        {/*
          Offered only for a running container, because freezing a stopped one
          means nothing. On by default: it costs milliseconds on a
          copy-on-write pool and is the whole difference between a snapshot
          that restores and one that might.
        */}
        {live && (
          <div className="mt-3 flex items-start gap-2">
            <Checkbox id="freeze" checked={freeze} onCheckedChange={(checked) => setFreeze(checked === true)} />
            <div>
              <Label htmlFor="freeze">{t('snapshots.freezeOption')}</Label>
              <p className="text-xs text-muted-foreground">{t('snapshots.freezeHint')}</p>
            </div>
          </div>
        )}
      </section>

      {deepest >= WARN_AT_CHAIN && (
        <div className="flex items-start gap-2 rounded-md bg-warning/10 p-3">
          <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
          <p className="text-sm text-foreground">{t('snapshots.chainWarning')}</p>
        </div>
      )}

      {snapshots.length === 0 ? (
        <p className="rounded-lg border border-border p-4 text-sm text-muted-foreground">{t('snapshots.empty')}</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('snapshots.columns.name')}</TableHead>
                <TableHead>{t('snapshots.columns.taken')}</TableHead>
                <TableHead>{t('snapshots.columns.mechanism')}</TableHead>
                <TableHead>{t('snapshots.columns.consistency')}</TableHead>
                <TableHead>{t('snapshots.columns.size')}</TableHead>
                <TableHead>{t('snapshots.columns.depth')}</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {snapshots.map((snapshot) => (
                <TableRow key={snapshot.id}>
                  <TableCell className="font-medium text-foreground">{snapshot.name}</TableCell>
                  <TableCell className="text-muted-foreground">{formatRelative(snapshot.createdAt)}</TableCell>
                  <TableCell>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Badge variant={snapshot.mechanism === 'rsync' ? 'warning' : 'secondary'}>
                          {snapshot.mechanism}
                        </Badge>
                      </TooltipTrigger>
                      <TooltipContent>
                        {t(`snapshots.mechanismHint.${snapshot.mechanism}`, snapshot.mechanism)}
                      </TooltipContent>
                    </Tooltip>
                  </TableCell>
                  <TableCell>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Badge variant={snapshot.wasFrozen ? 'success' : 'warning'}>
                          {snapshot.wasFrozen ? t('snapshots.consistent') : t('snapshots.crashConsistent')}
                        </Badge>
                      </TooltipTrigger>
                      <TooltipContent>
                        {snapshot.wasFrozen ? t('snapshots.consistentHint') : t('snapshots.crashConsistentHint')}
                      </TooltipContent>
                    </Tooltip>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{formatBytes(snapshot.sizeBytes)}</TableCell>
                  <TableCell className="text-muted-foreground">{snapshot.chainDepth}</TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t('actions.revert')}
                        permission={CT_PERM.MANAGE}
                        // Reverting the filesystem under a running init leaves
                        // processes holding inodes that no longer exist, and it
                        // does not fail cleanly.
                        disabled={live || !container.hostAvailable}
                        onClick={() => setReverting(snapshot)}
                      >
                        <IconRestore className="h-4 w-4" aria-hidden="true" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t('actions.delete')}
                        permission={CT_PERM.DELETE}
                        disabled={!container.hostAvailable}
                        onClick={() =>
                          deleteSnapshot.mutate({
                            id: container.id,
                            snapshotId: snapshot.id,
                          })
                        }
                      >
                        <IconTrash className="h-4 w-4" aria-hidden="true" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {reverting && (
        <ConfirmDialog
          open
          title={`${t('actions.revert')} — ${reverting.name}`}
          description={t('snapshots.revertWarning')}
          confirmLabel={t('actions.revert')}
          variant="destructive"
          onConfirm={() => {
            revertSnapshot.mutate({ id: container.id, snapshotId: reverting.id });
            setReverting(null);
          }}
          onOpenChange={(next) => {
            if (!next) setReverting(null);
          }}
        />
      )}
    </div>
  );
}
