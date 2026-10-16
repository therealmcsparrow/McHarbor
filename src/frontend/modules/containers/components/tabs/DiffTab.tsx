// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { useQuery } from '@tanstack/react-query';
import { IconPlus, IconPencil, IconMinus } from '@tabler/icons-react';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { Spinner } from '@resources/components/ui/Spinner';

type FilesystemChange = {
  path: string;
  kind: 'modified' | 'added' | 'deleted' | (string & {});
};

const DIFF_KINDS = ['added', 'modified', 'deleted'] as const;

type DiffTabProps = {
  containerId: string;
  isRunning: boolean;
};

function kindVariant(kind: string): 'success' | 'destructive' | 'secondary' {
  if (kind === 'added') return 'success';
  if (kind === 'deleted') return 'destructive';
  return 'secondary';
}

function kindIcon(kind: string) {
  if (kind === 'added') return IconPlus;
  if (kind === 'deleted') return IconMinus;
  return IconPencil;
}

export function DiffTab({ containerId }: DiffTabProps) {
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((s) => s.currentId);

  const query = useQuery({
    queryKey: ['container-diff', envId, containerId],
    queryFn: () =>
      api
        .get<FilesystemChange[]>(`/containers/${containerId}/diff`, envId ? { env: envId } : {})
        .then((r) => r.data ?? []),
    enabled: !!containerId,
  });

  const groups: Record<string, FilesystemChange[]> = {
    added: [],
    modified: [],
    deleted: [],
  };
  (query.data ?? []).forEach((c) => {
    if (c.kind in groups) (groups[c.kind] ?? []).push(c);
    else (groups.modified ?? []).push(c);
  });

  return (
    <div className="space-y-4">
      {query.isLoading ? (
        <div className="flex h-32 items-center justify-center">
          <Spinner size="md" />
        </div>
      ) : query.isError ? (
        <ErrorState variant="banner" title={t('diff.error')} error={query.error} />
      ) : !query.data || query.data.length === 0 ? (
        <div className="mc-glass flex h-32 items-center justify-center rounded-lg text-sm text-muted-foreground">
          {t('diff.empty')}
        </div>
      ) : (
        <>
          <div className="flex items-center gap-4 text-xs text-muted-foreground">
            <span>
              {t('diff.added')}: {(groups.added ?? []).length}
            </span>
            <span>
              {t('diff.modified')}: {(groups.modified ?? []).length}
            </span>
            <span>
              {t('diff.deleted')}: {(groups.deleted ?? []).length}
            </span>
          </div>
          {DIFF_KINDS.map((kind) => {
            const list = groups[kind] ?? [];
            if (list.length === 0) return null;
            const Icon = kindIcon(kind);
            const v = kindVariant(kind);
            return (
              <div key={kind} className="mc-glass rounded-lg p-4">
                <div className="mb-2 flex items-center gap-2">
                  <Icon
                    className={`h-4 w-4 ${v === 'success' ? 'text-success' : v === 'destructive' ? 'text-destructive' : 'text-muted-foreground'}`}
                  />
                  <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
                    {t(`diff.${kind}`)} ({list.length})
                  </h3>
                </div>
                <div className="max-h-96 overflow-y-auto">
                  <table className="w-full text-left text-xs">
                    <tbody>
                      {list.map((c) => (
                        <tr key={`${c.kind}:${c.path}`} className="border-b border-border last:border-0">
                          <td className="px-2 py-1.5 font-mono text-foreground">{c.path}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            );
          })}
        </>
      )}
    </div>
  );
}
