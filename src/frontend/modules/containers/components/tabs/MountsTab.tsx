// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { ContainerInspect } from '@core/types/docker';

type MountsTabProps = {
  container: ContainerInspect;
};

export function MountsTab({ container }: MountsTabProps) {
  const { t } = useTranslation('containers');
  const mounts = container.Mounts ?? [];

  if (mounts.length === 0) {
    return (
      <div className="rounded-lg border border-border bg-card p-6">
        <p className="text-sm text-muted-foreground">{t('mounts.noMounts')}</p>
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('detail.mounts')}</h3>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-xs">
          <thead>
            <tr className="border-b border-border">
              <th className="px-3 py-2 font-medium text-muted-foreground">{t('mounts.type')}</th>
              <th className="px-3 py-2 font-medium text-muted-foreground">{t('mounts.source')}</th>
              <th className="px-3 py-2 font-medium text-muted-foreground">{t('mounts.destination')}</th>
              <th className="px-3 py-2 font-medium text-muted-foreground">{t('mounts.mode')}</th>
              <th className="px-3 py-2 font-medium text-muted-foreground">{t('mounts.driver')}</th>
              <th className="px-3 py-2 font-medium text-muted-foreground">{t('mounts.propagation')}</th>
            </tr>
          </thead>
          <tbody>
            {mounts.map((m) => (
              <tr key={`${m.Type}:${m.Source ?? ''}:${m.Destination}`} className="border-b border-border last:border-0 hover:bg-muted/50">
                <td className="px-3 py-2">
                  <span className="rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium text-foreground">
                    {m.Type}
                  </span>
                </td>
                <td className="max-w-xs truncate px-3 py-2 font-mono text-foreground">{m.Source || '-'}</td>
                <td className="max-w-xs truncate px-3 py-2 font-mono text-foreground">{m.Destination}</td>
                <td className="px-3 py-2 text-foreground">{m.RW ? 'rw' : 'ro'}</td>
                <td className="px-3 py-2 text-foreground">{m.Driver || '-'}</td>
                <td className="px-3 py-2 text-foreground">{m.Propagation || '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
