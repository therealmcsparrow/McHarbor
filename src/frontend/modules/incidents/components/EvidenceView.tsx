// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Card } from '@resources/components/ui/Card';
import { Disclosure } from '@resources/components/ui/Disclosure';
import { formatBytes, formatDate } from '@resources/utils/format';
import type { Evidence, MetricPoint } from '@resources/hooks/useIncidents';
import { PluginEvidence } from './PluginEvidence';

function Section({ title, count, children }: { title: string; count: number; children: ReactNode }) {
  const { t } = useTranslation('incidents');
  return (
    <Disclosure title={title} trailing={<Badge variant="secondary">{count}</Badge>} defaultOpen={count > 0}>
      {count === 0 ? <p className="text-sm text-muted-foreground">{t('evidence.nothing')}</p> : children}
    </Disclosure>
  );
}

function Rows({ rows }: { rows: { key: string; cells: ReactNode[] }[] }) {
  return (
    <ul className="divide-y divide-border text-sm">
      {rows.map((r) => (
        <li key={r.key} className="flex flex-wrap gap-x-4 gap-y-1 py-1.5">
          {r.cells.map((c, i) => (
            <span
              key={`${r.key}-${String(i)}`}
              className={i === 0 ? 'w-40 shrink-0 text-xs text-muted-foreground' : 'break-all'}
            >
              {c}
            </span>
          ))}
        </li>
      ))}
    </ul>
  );
}

function peak(points: MetricPoint[]) {
  return points.reduce((max, p) => Math.max(max, p.value), 0);
}

function ContainerFacts({ evidence }: { evidence: Evidence }) {
  const { t } = useTranslation('incidents');
  const c = evidence.container;
  const facts: [string, ReactNode][] = [
    [t('evidence.image'), c.image ?? '—'],
    [t('evidence.state'), c.state ?? '—'],
    [t('evidence.exitCode'), c.exitCode ?? '—'],
    [t('evidence.restarts'), c.restarts ?? 0],
    [t('evidence.health'), c.health ?? '—'],
    [t('evidence.oomKilled'), c.oomKilled ? t('evidence.yes') : t('evidence.no')],
  ];
  return (
    <>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm md:grid-cols-3">
        {facts.map(([label, value]) => (
          <div key={label} className="flex gap-2">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="font-mono break-all">{value}</dd>
          </div>
        ))}
      </dl>
      <Section title={t('evidence.events')} count={evidence.events.length}>
        <Rows
          rows={evidence.events.map((e) => ({
            key: `${e.at}-${e.action}`,
            cells: [formatDate(e.at), e.action, e.exitCode ? t('evidence.exit', { code: e.exitCode }) : ''],
          }))}
        />
      </Section>
      <Section title={t('evidence.logs')} count={evidence.logs.length}>
        {evidence.logsNote && <p className="text-xs text-muted-foreground">{t(`logsNote.${evidence.logsNote}`)}</p>}
        <pre className="max-h-96 overflow-auto rounded-md bg-muted p-3 font-mono text-xs break-all whitespace-pre-wrap">
          {evidence.logs.join('\n')}
        </pre>
      </Section>
      <Section title={t('evidence.imageChanges')} count={evidence.imageChanges.length}>
        <Rows
          rows={evidence.imageChanges.map((i) => ({ key: i.at, cells: [formatDate(i.at), `${i.from} → ${i.to}`] }))}
        />
      </Section>
      <Section title={t('evidence.deploys')} count={evidence.deploys.length}>
        <Rows
          rows={evidence.deploys.map((d) => ({
            key: `${d.at}-${d.commit}`,
            cells: [formatDate(d.at), `${d.pipeline} / ${d.stage}`, d.commit, d.status],
          }))}
        />
      </Section>
      <Section title={t('evidence.drift')} count={evidence.drift.length}>
        <Rows
          rows={evidence.drift.map((d) => ({
            key: `${d.watch}-${d.field}`,
            cells: [d.watch, d.field, `${d.expected} → ${d.actual}`],
          }))}
        />
      </Section>
      <Section title={t('evidence.metrics')} count={evidence.containerCpu.length + evidence.hostCpu.length}>
        <dl className="grid grid-cols-2 gap-2 text-sm">
          <dt className="text-muted-foreground">{t('evidence.peakContainerCpu')}</dt>
          <dd>{peak(evidence.containerCpu).toFixed(1)}%</dd>
          <dt className="text-muted-foreground">{t('evidence.peakContainerMem')}</dt>
          <dd>{formatBytes(peak(evidence.containerMem))}</dd>
          <dt className="text-muted-foreground">{t('evidence.peakHostCpu')}</dt>
          <dd>{peak(evidence.hostCpu).toFixed(1)}%</dd>
          <dt className="text-muted-foreground">{t('evidence.peakHostMem')}</dt>
          <dd>{peak(evidence.hostMem).toFixed(1)}%</dd>
        </dl>
      </Section>
    </>
  );
}

export function EvidenceView({ evidence }: { evidence: Evidence }) {
  const { t } = useTranslation('incidents');
  return (
    <Card className="space-y-3 p-5">
      <div className="flex flex-wrap items-baseline gap-2">
        <h2 className="text-base font-semibold text-foreground">{t('evidence.title')}</h2>
        <span className="text-xs text-muted-foreground">
          {t('evidence.window', { from: formatDate(evidence.windowStart), to: formatDate(evidence.windowEnd) })}
        </span>
        {evidence.truncated && <Badge variant="warning">{t('evidence.truncated')}</Badge>}
      </div>
      {evidence.subject ? <PluginEvidence evidence={evidence} /> : <ContainerFacts evidence={evidence} />}
      <Section title={t('evidence.audit')} count={evidence.audit.length}>
        <Rows
          rows={evidence.audit.map((a) => ({
            key: `${a.at}-${a.action}`,
            cells: [formatDate(a.at), a.who, a.action, a.entity],
          }))}
        />
      </Section>
      <Section title={t('evidence.alerts')} count={evidence.alerts.length}>
        <Rows
          rows={evidence.alerts.map((a) => ({
            key: `${a.at}-${a.alert}`,
            cells: [formatDate(a.at), a.alert, a.state, a.message],
          }))}
        />
      </Section>
    </Card>
  );
}
