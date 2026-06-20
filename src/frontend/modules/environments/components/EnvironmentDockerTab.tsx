// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@resources/components/ui/Card';
import type { DockerSystemInfo } from '@core/types/docker';
import { useDockerInfo } from '@modules/docker/hooks/useDockerInfo';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between border-b border-border py-2 last:border-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="max-w-[60%] break-all text-right text-sm text-foreground">{value}</span>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold text-foreground">{title}</h3>
      <div className="rounded-lg border border-border bg-muted/30 px-4">{children}</div>
    </div>
  );
}

type EnvironmentDockerTabProps = {
  envId: string;
};

export function EnvironmentDockerTab({ envId }: EnvironmentDockerTabProps) {
  const { t } = useTranslation('environments');
  const enabled = !!envId;
  const { data: info, isLoading } = useDockerInfo(enabled);

  if (!enabled) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('detail.docker.title')}</CardTitle>
          <CardDescription>{t('detail.docker.unavailable')}</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!info) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('detail.docker.title')}</CardTitle>
          <CardDescription>{t('detail.docker.unavailable')}</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.docker.title')}</CardTitle>
        <CardDescription>{t('detail.docker.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <Section title={t('detail.docker.server')}>
            <InfoRow label={t('detail.docker.version')} value={info.serverVersion} />
            <InfoRow label={t('detail.docker.apiVersion')} value={info.apiVersion} />
            <InfoRow label={t('detail.docker.minApiVersion')} value={info.minApiVersion} />
            <InfoRow label={t('detail.docker.gitCommit')} value={info.gitCommit} />
            <InfoRow label={t('detail.docker.goVersion')} value={info.goVersion} />
            <InfoRow label={t('detail.docker.os')} value={info.os} />
            <InfoRow label={t('detail.docker.architecture')} value={info.architecture} />
            <InfoRow label={t('detail.docker.kernelVersion')} value={info.kernelVersion} />
            <InfoRow label={t('detail.docker.hostname')} value={info.hostname} />
            <InfoRow
              label={t('detail.docker.serverId')}
              value={<code className="text-xs">{info.id}</code>}
            />
          </Section>

          <Section title={t('detail.docker.resources')}>
            <InfoRow label={t('detail.docker.cpus')} value={info.ncpu} />
            <InfoRow label={t('detail.docker.memory')} value={formatBytes(info.memTotal)} />
          </Section>

          <Section title={t('detail.docker.counts')}>
            <InfoRow label={t('detail.docker.containers')} value={info.containers} />
            <InfoRow
              label={t('detail.docker.containersRunning')}
              value={info.containersRunning}
            />
            <InfoRow
              label={t('detail.docker.containersPaused')}
              value={info.containersPaused}
            />
            <InfoRow
              label={t('detail.docker.containersStopped')}
              value={info.containersStopped}
            />
            <InfoRow label={t('detail.docker.images')} value={info.images} />
          </Section>

          <Section title={t('detail.docker.storage')}>
            <InfoRow label={t('detail.docker.storageDriver')} value={info.storageDriver} />
            <InfoRow
              label={t('detail.docker.dockerRootDir')}
              value={<code className="text-xs">{info.dockerRootDir}</code>}
            />
            <InfoRow label={t('detail.docker.loggingDriver')} value={info.loggingDriver} />
            {info.driverStatus.map((pair, i) => (
              <InfoRow
                key={`ds-${pair[0] ?? i}-${i}`}
                label={pair[0] ?? ''}
                value={pair[1] ?? ''}
              />
            ))}
          </Section>

          <Section title={t('detail.docker.runtime')}>
            <InfoRow label={t('detail.docker.cgroupDriver')} value={info.cgroupDriver} />
            <InfoRow label={t('detail.docker.cgroupVersion')} value={info.cgroupVersion} />
            <InfoRow label={t('detail.docker.defaultRuntime')} value={info.defaultRuntime} />
            <InfoRow
              label={t('detail.docker.runtimes')}
              value={
                <div className="flex flex-wrap justify-end gap-1">
                  {info.runtimes.map((r) => (
                    <Badge key={r} variant="secondary">
                      {r}
                    </Badge>
                  ))}
                </div>
              }
            />
            {info.isolation && (
              <InfoRow label={t('detail.docker.isolation')} value={info.isolation} />
            )}
          </Section>

          <Section title={t('detail.docker.security')}>
            {info.securityOptions.map((opt) => (
              <InfoRow
                key={opt}
                label={opt.split('=')[0] ?? opt}
                value={opt.split('=').slice(1).join('=') || '-'}
              />
            ))}
          </Section>

          <Section title={t('detail.docker.plugins')}>
            <InfoRow
              label={t('detail.docker.volumePlugins')}
              value={info.pluginsVolume.join(', ') || '-'}
            />
            <InfoRow
              label={t('detail.docker.networkPlugins')}
              value={info.pluginsNetwork.join(', ') || '-'}
            />
            <InfoRow
              label={t('detail.docker.logPlugins')}
              value={info.pluginsLog.join(', ') || '-'}
            />
          </Section>

          <Section title={t('detail.docker.swarm')}>
            <InfoRow
              label={t('detail.docker.status')}
              value={
                <Badge variant={info.swarmActive ? 'default' : 'secondary'}>
                  {info.swarmActive
                    ? t('detail.docker.swarmActive')
                    : t('detail.docker.swarmInactive')}
                </Badge>
              }
            />
            {info.swarmActive && (
              <>
                <InfoRow label={t('detail.docker.swarmNodeId')} value={info.swarmNodeId} />
                <InfoRow label={t('detail.docker.swarmManagers')} value={info.swarmManagers} />
                <InfoRow label={t('detail.docker.swarmNodes')} value={info.swarmNodes} />
              </>
            )}
          </Section>

          {info.labels.length > 0 && (
            <Section title={t('detail.docker.labels')}>
              {info.labels.map((label) => {
                const [key, ...rest] = label.split('=');
                return (
                  <InfoRow
                    key={label}
                    label={key ?? label}
                    value={rest.join('=') || '-'}
                  />
                );
              })}
            </Section>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export type { DockerSystemInfo };
