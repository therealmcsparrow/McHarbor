// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';

type ExecHistoryEntry = {
  id: string;
  command: string[];
  workDir?: string;
  user?: string;
  exitCode: number;
  stdout: string;
  stderr: string;
  durationMs: number;
  createdAt?: string;
};

export function execHistoryQueryKey(envId: string | null | undefined, containerId: string) {
  return ['container-exec-history', envId, containerId];
}

export function ExecHistoryPanel({ containerId }: { containerId: string }) {
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((s) => s.currentId);
  const execHistory = useQuery({
    queryKey: execHistoryQueryKey(envId, containerId),
    queryFn: () =>
      api
        .get<ExecHistoryEntry[]>(`/containers/${containerId}/exec/history?limit=50`, envId ? { env: envId } : {})
        .then(assertSuccess)
        .then((rows) => rows ?? []),
    enabled: !!containerId,
  });

  return (
    <div className="mc-glass rounded-lg p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">{t('exec.history')}</h3>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => execHistory.refetch()}
          disabled={execHistory.isFetching}
          aria-label={t('exec.refreshHistory')}
        >
          {t('exec.refreshHistory')}
        </Button>
      </div>
      {execHistory.isLoading ? (
        <div className="flex h-20 items-center justify-center">
          <Spinner size="sm" />
        </div>
      ) : execHistory.isError ? (
        <ErrorState variant="card" error={execHistory.error} onRetry={() => void execHistory.refetch()} />
      ) : execHistory.data && execHistory.data.length > 0 ? (
        <div className="max-h-96 overflow-y-auto">
          <table className="w-full text-left text-xs">
            <thead className="sticky top-0 bg-muted">
              <tr className="border-b border-border">
                <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('exec.columns.command')}</th>
                <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('exec.columns.exit')}</th>
                <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('exec.columns.duration')}</th>
              </tr>
            </thead>
            <tbody>
              {execHistory.data.map((h) => (
                <tr key={h.id} className="border-b border-border last:border-0 hover:bg-muted/50">
                  <td className="px-2 py-1.5 font-mono text-foreground">
                    {Array.isArray(h.command) ? h.command.join(' ') : ''}
                  </td>
                  <td className={`px-2 py-1.5 ${h.exitCode === 0 ? 'text-success' : 'text-destructive'}`}>
                    {h.exitCode}
                  </td>
                  <td className="px-2 py-1.5 text-muted-foreground">{t('exec.durationMs', { ms: h.durationMs })}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          <IconTrash className="inline h-3 w-3" /> {t('exec.emptyHistory')}
        </p>
      )}
    </div>
  );
}
