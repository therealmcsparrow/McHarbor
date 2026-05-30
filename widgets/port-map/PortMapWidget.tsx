// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useContainers } from '@modules/containers/hooks/useContainers';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

type PortEntry = {
  containerId: string;
  containerName: string;
  hostPort: number | undefined;
  containerPort: number;
  protocol: string;
};

export default function PortMapWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: containers, isLoading } = useContainers(true);

  const portEntries = useMemo(() => {
    if (!containers) return [];
    const entries: PortEntry[] = [];
    for (const c of containers) {
      const name = (c.Names?.[0] ?? '').replace(/^\//, '');
      for (const p of c.Ports ?? []) {
        if (p.PublicPort) {
          entries.push({
            containerId: c.Id,
            containerName: name,
            hostPort: p.PublicPort,
            containerPort: p.PrivatePort,
            protocol: p.Type,
          });
        }
      }
    }
    return entries.sort((a, b) => (a.hostPort ?? 0) - (b.hostPort ?? 0)).slice(0, 20);
  }, [containers]);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('loading')}
      </div>
    );
  }

  if (portEntries.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('portMapWidget.noPorts')}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-2 text-sm font-semibold text-foreground">
        {t('portMapWidget.title')}
      </h3>
      <div className="flex-1 overflow-y-auto px-4 pb-2">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground">
              <th className="pb-1.5 font-medium">{t('portMapWidget.container')}</th>
              <th className="pb-1.5 font-medium">{t('portMapWidget.mapping')}</th>
              <th className="pb-1.5 font-medium">{t('portMapWidget.protocol')}</th>
            </tr>
          </thead>
          <tbody>
            {portEntries.map((p, i) => (
              <tr key={`${p.containerId}-${p.containerPort}-${i}`} className="border-b border-border/50 last:border-0">
                <td className="max-w-[120px] truncate py-1.5 pr-2 text-foreground">
                  {p.containerName}
                </td>
                <td className="py-1.5 pr-2 font-mono text-muted-foreground">
                  {p.hostPort} → {p.containerPort}
                </td>
                <td className="py-1.5 uppercase text-muted-foreground">{p.protocol}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
