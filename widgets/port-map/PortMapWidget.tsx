// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconPlugConnectedX } from '@tabler/icons-react';
import { useContainers } from '@modules/containers/hooks/useContainers';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import {
  ROW_HEIGHT,
  ToneDot,
  WidgetBadge,
  WidgetEmpty,
  WidgetFrame,
  WidgetList,
  WidgetLoading,
  WidgetRow,
  useFittedRows,
} from '@modules/dashboard/components/widget-kit';

type PortEntry = {
  containerId: string;
  containerName: string;
  running: boolean;
  hostIp: string;
  hostPort: number;
  containerPort: number;
  protocol: string;
};

export default function PortMapWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: containers, isLoading } = useContainers(true);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const entries = useMemo(() => {
    const out: PortEntry[] = [];
    const seen = new Set<string>();
    for (const c of containers ?? []) {
      const label = (c.Names?.[0] ?? '').replace(/^\//, '');
      for (const p of c.Ports ?? []) {
        if (!p.PublicPort) continue;
        // Docker lists a port once per bound address; the map wants it once.
        const key = `${c.Id}:${p.PublicPort}:${p.PrivatePort}:${p.Type}`;
        if (seen.has(key)) continue;
        seen.add(key);
        out.push({
          containerId: c.Id,
          containerName: label,
          running: c.State === 'running',
          hostIp: p.IP ?? '',
          hostPort: p.PublicPort,
          containerPort: p.PrivatePort,
          protocol: p.Type,
        });
      }
    }
    return out.sort((a, b) => a.hostPort - b.hostPort);
  }, [containers]);

  const shown = entries.slice(0, capacity);
  const remaining = entries.length - shown.length;

  return (
    <WidgetFrame
      title={t('portMapWidget.title')}
      badge={
        entries.length > 0 ? (
          <WidgetBadge>{t('portMapWidget.published', { count: entries.length })}</WidgetBadge>
        ) : undefined
      }
      footer={remaining > 0 ? <span>{t('portMapWidget.more', { count: remaining })}</span> : undefined}
    >
      {isLoading ? (
        <WidgetLoading label={t('loading')} />
      ) : entries.length === 0 ? (
        <WidgetEmpty
          icon={<IconPlugConnectedX className="size-5" aria-hidden="true" />}
          title={t('portMapWidget.noPorts')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((p) => (
            <li key={`${p.containerId}-${p.hostPort}-${p.containerPort}-${p.protocol}`}>
              <WidgetRow
                leading={
                  <span className="w-12 text-right font-mono text-sm font-semibold text-foreground tabular-nums">
                    {p.hostPort}
                  </span>
                }
                title={p.containerName}
                subtitle={p.hostIp && p.hostIp !== '0.0.0.0' && p.hostIp !== '::' ? p.hostIp : undefined}
                trailing={
                  <>
                    <span className="font-mono tabular-nums">
                      &rarr; {p.containerPort}
                      <span className="ml-1 text-muted-foreground/70 uppercase">{p.protocol}</span>
                    </span>
                    <ToneDot tone={p.running ? 'success' : 'neutral'} />
                  </>
                }
                onClick={() => navigate(`/containers/${p.containerId}`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
