// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import { Badge } from '@resources/components/ui/Badge';
import { useDockerInfo } from '../hooks/useDockerInfo';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between py-2 border-b border-border last:border-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm text-foreground text-right max-w-[60%] break-all">{value}</span>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="text-sm font-semibold text-foreground mb-2">{title}</h3>
      <div className="rounded-lg border border-border bg-muted/30 px-4">{children}</div>
    </div>
  );
}

export function DockerInfoTab() {
  const { t } = useTranslation('docker');
  const { data: info, isLoading } = useDockerInfo();

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!info) return null;

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <Section title={t('info.server')}>
        <InfoRow label={t('info.version')} value={info.serverVersion} />
        <InfoRow label={t('info.apiVersion')} value={info.apiVersion} />
        <InfoRow label={t('info.minApiVersion')} value={info.minApiVersion} />
        <InfoRow label={t('info.gitCommit')} value={info.gitCommit} />
        <InfoRow label={t('info.goVersion')} value={info.goVersion} />
        <InfoRow label={t('info.os')} value={info.os} />
        <InfoRow label={t('info.architecture')} value={info.architecture} />
        <InfoRow label={t('info.kernelVersion')} value={info.kernelVersion} />
        <InfoRow label={t('info.hostname')} value={info.hostname} />
        <InfoRow label={t('info.serverId')} value={<code className="text-xs">{info.id}</code>} />
      </Section>

      <Section title={t('info.resources')}>
        <InfoRow label={t('info.cpus')} value={info.ncpu} />
        <InfoRow label={t('info.memory')} value={formatBytes(info.memTotal)} />
      </Section>

      <Section title={t('info.counts')}>
        <InfoRow label={t('info.containers')} value={info.containers} />
        <InfoRow label={t('info.containersRunning')} value={info.containersRunning} />
        <InfoRow label={t('info.containersPaused')} value={info.containersPaused} />
        <InfoRow label={t('info.containersStopped')} value={info.containersStopped} />
        <InfoRow label={t('info.images')} value={info.images} />
      </Section>

      <Section title={t('info.storage')}>
        <InfoRow label={t('info.storageDriver')} value={info.storageDriver} />
        <InfoRow label={t('info.dockerRootDir')} value={<code className="text-xs">{info.dockerRootDir}</code>} />
        <InfoRow label={t('info.loggingDriver')} value={info.loggingDriver} />
        {info.driverStatus.map((pair, i) => (
          <InfoRow key={`ds-${pair[0] ?? i}-${i}`} label={pair[0] ?? ''} value={pair[1] ?? ''} />
        ))}
      </Section>

      <Section title={t('info.runtime')}>
        <InfoRow label={t('info.cgroupDriver')} value={info.cgroupDriver} />
        <InfoRow label={t('info.cgroupVersion')} value={info.cgroupVersion} />
        <InfoRow label={t('info.defaultRuntime')} value={info.defaultRuntime} />
        <InfoRow
          label={t('info.runtimes')}
          value={
            <div className="flex flex-wrap gap-1 justify-end">
              {info.runtimes.map((r) => (
                <Badge key={r} variant="secondary">{r}</Badge>
              ))}
            </div>
          }
        />
        {info.isolation && <InfoRow label={t('info.isolation')} value={info.isolation} />}
      </Section>

      <Section title={t('info.security')}>
        {info.securityOptions.map((opt) => (
          <InfoRow key={opt} label={opt.split('=')[0] ?? opt} value={opt.split('=').slice(1).join('=') || '-'} />
        ))}
      </Section>

      <Section title={t('info.plugins')}>
        <InfoRow
          label={t('info.volumePlugins')}
          value={info.pluginsVolume.join(', ') || '-'}
        />
        <InfoRow
          label={t('info.networkPlugins')}
          value={info.pluginsNetwork.join(', ') || '-'}
        />
        <InfoRow
          label={t('info.logPlugins')}
          value={info.pluginsLog.join(', ') || '-'}
        />
      </Section>

      <Section title={t('info.swarm')}>
        <InfoRow
          label={t('info.status')}
          value={
            <Badge variant={info.swarmActive ? 'default' : 'secondary'}>
              {info.swarmActive ? t('info.swarmActive') : t('info.swarmInactive')}
            </Badge>
          }
        />
        {info.swarmActive && (
          <>
            <InfoRow label={t('info.swarmNodeId')} value={info.swarmNodeId} />
            <InfoRow label={t('info.swarmManagers')} value={info.swarmManagers} />
            <InfoRow label={t('info.swarmNodes')} value={info.swarmNodes} />
          </>
        )}
      </Section>

      {info.labels.length > 0 && (
        <Section title={t('info.labels')}>
          {info.labels.map((label) => {
            const [key, ...rest] = label.split('=');
            return <InfoRow key={label} label={key ?? label} value={rest.join('=') || '-'} />;
          })}
        </Section>
      )}
    </div>
  );
}
