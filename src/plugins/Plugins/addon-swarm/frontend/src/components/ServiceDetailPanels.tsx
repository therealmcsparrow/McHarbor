// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { DetailPanel, DetailRow, KeyValueList, StringList } from '../components/DetailTabs';
import { useSwarmServiceLogs } from '../hooks/useSwarmServices';
import { ServiceEnvList } from './ServiceEnvList';
import { formatBytes, formatCpus, portsText, replicaText, shortId } from '../format';
import { formatDateTime } from '../date-format';
import type { SwarmServiceDetail } from '../types';

const READ_ONLY_SUFFIX = ' (ro)';
export function OverviewPanel({ service }: { service: SwarmServiceDetail }) {
  const { t } = useTranslation('swarm');
  const ports = portsText(service);

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <DetailPanel title={t('detail.panels.identity')}>
        <DetailRow label={t('columns.id')}>
          <code className="font-mono text-xs">{shortId(service.id)}</code>
        </DetailRow>
        <DetailRow label={t('columns.image')}>
          <span className="font-mono text-xs break-all">{service.image}</span>
        </DetailRow>
        <DetailRow label={t('columns.mode')}>{t(`serviceMode.${service.mode}`)}</DetailRow>
        <DetailRow label={t('columns.replicas')}>{replicaText(service)}</DetailRow>
        <DetailRow label={t('columns.created')}>{formatDateTime(service.createdAt)}</DetailRow>
        <DetailRow label={t('columns.updated')}>{formatDateTime(service.updatedAt)}</DetailRow>
      </DetailPanel>

      <DetailPanel title={t('detail.panels.networking')}>
        <DetailRow label={t('columns.ports')}>{ports || <span className="text-muted-foreground">—</span>}</DetailRow>
        <DetailRow label={t('columns.networks')}>
          <StringList items={service.networks} />
        </DetailRow>
        <DetailRow label={t('detail.fields.endpointMode')}>
          {service.endpointMode || <span className="text-muted-foreground">—</span>}
        </DetailRow>
      </DetailPanel>

      <DetailPanel title={t('detail.panels.resources')}>
        <DetailRow label={t('detail.fields.cpuLimit')}>{formatCpus(service.resources.limitNanoCpus)}</DetailRow>
        <DetailRow label={t('detail.fields.memoryLimit')}>{formatBytes(service.resources.limitMemoryBytes)}</DetailRow>
        <DetailRow label={t('detail.fields.cpuReservation')}>{formatCpus(service.resources.reserveNanoCpus)}</DetailRow>
        <DetailRow label={t('detail.fields.memoryReservation')}>
          {formatBytes(service.resources.reserveMemoryBytes)}
        </DetailRow>
        <DetailRow label={t('detail.fields.constraints')}>
          <StringList items={service.constraints} />
        </DetailRow>
      </DetailPanel>

      <DetailPanel title={t('detail.panels.rollout')}>
        <DetailRow label={t('detail.fields.parallelism')}>{service.updateConfig.parallelism || '—'}</DetailRow>
        <DetailRow label={t('detail.fields.updateDelay')}>
          {service.updateConfig.delaySeconds ? `${service.updateConfig.delaySeconds}s` : '—'}
        </DetailRow>
        <DetailRow label={t('detail.fields.failureAction')}>{service.updateConfig.failureAction || '—'}</DetailRow>
        <DetailRow label={t('detail.fields.order')}>{service.updateConfig.order || '—'}</DetailRow>
        <DetailRow label={t('detail.fields.restartCondition')}>{service.restartPolicy.condition || '—'}</DetailRow>
      </DetailPanel>
    </div>
  );
}

export function ConfigPanel({ service }: { service: SwarmServiceDetail }) {
  const { t } = useTranslation('swarm');

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <DetailPanel title={t('detail.panels.command')}>
        <DetailRow label={t('detail.fields.command')}>
          <StringList items={service.command} />
        </DetailRow>
        <DetailRow label={t('detail.fields.args')}>
          <StringList items={service.args} />
        </DetailRow>
        <DetailRow label={t('detail.fields.env')}>
          <ServiceEnvList serviceId={service.id} env={service.env} />
        </DetailRow>
      </DetailPanel>

      <DetailPanel title={t('detail.panels.mounts')}>
        {service.mounts && service.mounts.length > 0 ? (
          service.mounts.map((mount) => (
            <DetailRow key={`${mount.source}:${mount.target}`} label={mount.type}>
              <span className="font-mono text-xs break-all">
                {mount.source || '—'} → {mount.target}
                {mount.readOnly ? READ_ONLY_SUFFIX : ''}
              </span>
            </DetailRow>
          ))
        ) : (
          <DetailRow label={t('detail.fields.mounts')}>
            <span className="text-muted-foreground">—</span>
          </DetailRow>
        )}
      </DetailPanel>

      <DetailPanel title={t('detail.panels.secrets')}>
        {/* Names and mount paths only — the payload is not readable through
            any endpoint this module has. */}
        <DetailRow label={t('detail.fields.secrets')}>
          <StringList items={service.secrets?.map((s) => `${s.name} → ${s.target}`)} />
        </DetailRow>
        <DetailRow label={t('detail.fields.configs')}>
          <StringList items={service.configs?.map((c) => `${c.name} → ${c.target}`)} />
        </DetailRow>
      </DetailPanel>
    </div>
  );
}

export function LabelsPanel({ service }: { service: SwarmServiceDetail }) {
  const { t } = useTranslation('swarm');

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <DetailPanel title={t('detail.panels.serviceLabels')}>
        <DetailRow label={t('columns.labels')}>
          <KeyValueList entries={service.labels} />
        </DetailRow>
      </DetailPanel>
      <DetailPanel title={t('detail.panels.containerLabels')}>
        <DetailRow label={t('columns.labels')}>
          <KeyValueList entries={service.containerLabels} />
        </DetailRow>
      </DetailPanel>
    </div>
  );
}

export function LogsPanel({ id }: { id: string }) {
  const { t } = useTranslation('swarm');
  const logs = useSwarmServiceLogs(id, 500);

  if (logs.isLoading) return <CardSkeleton lines={10} />;
  if (logs.error) return <ErrorState title={t('errors.logsFailed')} error={logs.error} />;
  if (!logs.data) return <EmptyState variant="compact" title={t('detail.logsEmpty')} />;

  return (
    <pre className="h-full overflow-auto rounded-xl bg-muted p-4 font-mono text-xs text-foreground">{logs.data}</pre>
  );
}
