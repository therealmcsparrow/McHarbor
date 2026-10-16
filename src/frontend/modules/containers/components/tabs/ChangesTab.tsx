// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useAuditLogs } from '@resources/hooks/useAuditLogs';
import { Spinner } from '@resources/components/ui/Spinner';
import type { AuditEntry } from '@resources/components/AuditEntryDetails';

export function ChangesTab({ containerId }: { containerId: string }) {
  const { t } = useTranslation('containers');
  const query = useAuditLogs('', 'container', { from: null, to: null }, containerId);

  const entries: AuditEntry[] = query.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <div className="space-y-3">
      <div className="mc-glass rounded-lg p-4 text-xs text-muted-foreground">{t('changes.scope')}</div>
      {query.isLoading ? (
        <div className="flex h-32 items-center justify-center">
          <Spinner size="md" />
        </div>
      ) : entries.length === 0 ? (
        <div className="mc-glass flex h-32 items-center justify-center rounded-lg text-sm text-muted-foreground">
          {t('changes.empty')}
        </div>
      ) : (
        <div className="mc-glass overflow-x-auto rounded-lg">
          <table className="w-full text-left text-xs">
            <thead className="bg-muted">
              <tr className="border-b border-border">
                <th className="px-3 py-2 font-medium text-muted-foreground">{t('changes.columns.time')}</th>
                <th className="px-3 py-2 font-medium text-muted-foreground">{t('changes.columns.action')}</th>
                <th className="px-3 py-2 font-medium text-muted-foreground">{t('changes.columns.user')}</th>
                <th className="px-3 py-2 font-medium text-muted-foreground">{t('changes.columns.details')}</th>
              </tr>
            </thead>
            <tbody>
              {entries.slice(0, 100).map((e) => (
                <tr key={e.id} className="border-b border-border last:border-0 hover:bg-muted/50">
                  <td className="px-3 py-1.5 text-muted-foreground">
                    {new Date(e.createdAt).toISOString().replace('T', ' ').slice(0, 19)}
                  </td>
                  <td className="px-3 py-1.5 font-mono text-foreground">{e.action}</td>
                  <td className="px-3 py-1.5 text-foreground">{e.username ?? e.userId ?? ''}</td>
                  <td className="px-3 py-1.5 text-foreground">{e.details ?? ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
