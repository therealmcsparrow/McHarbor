// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { useContainerProcesses } from '../hooks/useContainers';
import { formatBytes } from '../format';
import { isLive, type Container } from '../types';

/**
 * What is running inside a container.
 *
 * A system container has this and a virtual machine does not: the host can
 * enumerate the guest's processes by reading its cgroup, with nothing inside
 * the container cooperating and no agent installed. That is worth a tab of its
 * own — for a VM the same information needs a guest agent that may not be
 * there, which is why addon-kvm has no equivalent.
 *
 * Command strings come from inside the container and are therefore untrusted.
 * They are rendered as text, never as markup, and never used to build a link.
 */
export function ProcessesPanel({ container }: { container: Container }) {
  const { t } = useTranslation('ct');
  const live = isLive(container);
  const { data, isLoading } = useContainerProcesses(container.id, live);

  if (!live) {
    return <p className="rounded-lg border border-border p-4 text-sm text-muted-foreground">{t('processes.empty')}</p>;
  }
  if (isLoading) return <Skeleton className="h-64 w-full" />;

  const processes = data ?? [];

  return (
    <div className="space-y-3">
      <div className="overflow-x-auto rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('processes.columns.pid')}</TableHead>
              <TableHead>{t('processes.columns.user')}</TableHead>
              <TableHead>{t('processes.columns.command')}</TableHead>
              <TableHead>{t('processes.columns.cpu')}</TableHead>
              <TableHead>{t('processes.columns.memory')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {processes.map((process) => (
              <TableRow key={`${process.pid}-${process.hostPid ?? 0}`}>
                <TableCell>
                  {/*
                    The pid inside the container, with the host's own pid for
                    the same process behind a tooltip. They are different
                    numbers, and an operator moving between this table and
                    `ps` on the host needs to know which one they are holding.
                  */}
                  {process.hostPid ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="cursor-help font-mono text-xs text-foreground">{process.pid}</span>
                      </TooltipTrigger>
                      <TooltipContent>
                        {t('processes.hostPidHint')} — {process.hostPid}
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <span className="font-mono text-xs text-foreground">{process.pid}</span>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">{process.user || '—'}</TableCell>
                <TableCell>
                  <span className="font-mono text-xs text-foreground">{process.command}</span>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {process.cpuPercent !== undefined ? `${Math.round(process.cpuPercent)}%` : '—'}
                </TableCell>
                <TableCell className="text-muted-foreground">{formatBytes(process.memoryBytes)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <p className="text-xs text-muted-foreground">{t('processes.note')}</p>
    </div>
  );
}
