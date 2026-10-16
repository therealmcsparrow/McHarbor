// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArchive, IconCopyPlus } from '@tabler/icons-react';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { CT_PERM } from '../permissions';
import { useOrphanedBackups } from '../hooks/useRecreate';
import type { BackupRun } from '../hooks/useCTBackups';
import { formatBytes, formatRelative } from '../format';
import { RecreateFromBackupDialog } from './RecreateFromBackupDialog';

interface OrphanGroup {
  containerId: string;
  containerName: string;
  environmentId: string;
  runs: BackupRun[];
}

function groupByContainer(runs: BackupRun[]): OrphanGroup[] {
  const groups = new Map<string, OrphanGroup>();
  for (const run of runs) {
    const group = groups.get(run.containerId) ?? {
      containerId: run.containerId,
      containerName: run.containerName ?? '',
      environmentId: run.environmentId,
      runs: [],
    };
    group.runs.push(run);
    groups.set(run.containerId, group);
  }
  return [...groups.values()];
}

export function OrphanedBackupsButton({
  environments,
  environmentId,
  onRecreated,
}: {
  environments: { id: string; name: string }[];
  environmentId?: string;
  onRecreated: (containerId: string) => void;
}) {
  const { t } = useTranslation('ct');
  const [listOpen, setListOpen] = useState(false);
  const [recreating, setRecreating] = useState<BackupRun | null>(null);
  const orphans = useOrphanedBackups(environmentId, listOpen);
  const groups = useMemo(() => groupByContainer(orphans.data ?? []), [orphans.data]);
  const manageGate = usePermissionGate(CT_PERM.MANAGE);
  const recreatePermission = manageGate.denied ? CT_PERM.MANAGE : CT_PERM.RESTORE;
  const hostName = (id: string) => environments.find((env) => env.id === id)?.name ?? id;

  return (
    <>
      <Button size="sm" variant="outline" permission={CT_PERM.BACKUP} onClick={() => setListOpen(true)}>
        <IconArchive className="mr-1.5 h-4 w-4" aria-hidden="true" />
        {t('recreate.orphaned.button')}
      </Button>

      {listOpen && !recreating && (
        <Dialog open onOpenChange={setListOpen}>
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>{t('recreate.orphaned.title')}</DialogTitle>
              <DialogDescription>{t('recreate.orphaned.description')}</DialogDescription>
            </DialogHeader>
            {orphans.isLoading && <Spinner className="h-5 w-5" />}
            {orphans.isError && (
              <p role="alert" className="text-sm break-words text-destructive">
                {orphans.error.message}
              </p>
            )}
            {orphans.isSuccess && groups.length === 0 && (
              <p className="text-sm text-muted-foreground">{t('recreate.orphaned.empty')}</p>
            )}
            <div className="max-h-[60vh] space-y-4 overflow-y-auto">
              {groups.map((group) => (
                <section key={group.containerId} className="rounded-md border border-border">
                  <header className="flex flex-wrap items-baseline justify-between gap-2 border-b border-border px-3 py-2">
                    <span className="font-medium text-foreground">
                      {group.containerName || t('recreate.orphaned.unknownName')}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {hostName(group.environmentId)} · <span className="font-mono">{group.containerId}</span>
                    </span>
                  </header>
                  <ul className="divide-y divide-border">
                    {group.runs.map((run) => (
                      <li key={run.id} className="flex items-center justify-between gap-2 px-3 py-2 text-sm">
                        <span className="flex items-center gap-2">
                          {formatRelative(run.startedAt)}
                          <Badge variant={run.kind === 'full' ? 'secondary' : 'outline'}>
                            {t(`backups.kind.${run.kind}`)}
                          </Badge>
                          <span className="text-muted-foreground">{formatBytes(run.bytesTotal)}</span>
                        </span>
                        <Button
                          size="sm"
                          variant="outline"
                          permission={recreatePermission}
                          onClick={() => setRecreating(run)}
                        >
                          <IconCopyPlus className="mr-1.5 h-4 w-4" aria-hidden="true" />
                          {t('recreate.action')}
                        </Button>
                      </li>
                    ))}
                  </ul>
                </section>
              ))}
            </div>
          </DialogContent>
        </Dialog>
      )}

      {recreating && (
        <RecreateFromBackupDialog
          run={recreating}
          environments={environments}
          onOpenChange={(next) => {
            if (!next) setRecreating(null);
          }}
          onCreated={(containerId) => {
            setRecreating(null);
            setListOpen(false);
            onRecreated(containerId);
          }}
        />
      )}
    </>
  );
}
