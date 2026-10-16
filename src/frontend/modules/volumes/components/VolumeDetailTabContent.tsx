// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// VolumeDetailTabContent — what the volume tab panel shows, one face at a
// time. Section grammar copied from the container overview: `mc-glass`
// blocks, uppercase tracked headings, `InfoRow` for label/value pairs.

import { Badge } from '@resources/components/ui/Badge';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { formatBytes, formatDate } from '@resources/utils/format';
import type { VolumeInfo } from '@core/types/docker';
import type { VolumeTabId } from './VolumeDetailTabs';
import { ConfigurationPanel, ContainersPanel } from './VolumeDetailPanels';

/** One container's use of this volume, flattened from its mount entry. */
export type VolumeMount = {
  containerId: string;
  containerName: string;
  state: string;
  destination: string;
  readWrite: boolean;
};

type VolumeDetailTabContentProps = {
  activeTab: VolumeTabId;
  volume: VolumeInfo;
  mounts: VolumeMount[];
  t: (key: string) => string;
  onContainerOpen: (containerID: string) => void;
};

export function VolumeDetailTabContent({ activeTab, volume, mounts, t, onContainerOpen }: VolumeDetailTabContentProps) {
  if (activeTab === 'containers') {
    return <ContainersPanel mounts={mounts} t={t} onContainerOpen={onContainerOpen} />;
  }

  if (activeTab === 'configuration') {
    return <ConfigurationPanel volume={volume} t={t} />;
  }

  return <OverviewPanel volume={volume} mountCount={mounts.length} t={t} />;
}

function OverviewPanel({
  volume,
  mountCount,
  t,
}: {
  volume: VolumeInfo;
  mountCount: number;
  t: (key: string) => string;
}) {
  const size = volume.UsageData?.Size;

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <div className="mc-glass rounded-lg p-6">
        <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
          {t('detail.general')}
        </h3>
        <InfoRow label={t('detail.name')}>{volume.Name}</InfoRow>
        <InfoRow label={t('detail.driver')}>
          <Badge variant="default" className="px-1.5 py-0 text-[10px]">
            {volume.Driver}
          </Badge>
        </InfoRow>
        <InfoRow label={t('detail.scope')}>
          <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
            {volume.Scope || '-'}
          </Badge>
        </InfoRow>
        <InfoRow label={t('detail.mountpoint')}>
          <span className="font-mono text-xs">{volume.Mountpoint}</span>
        </InfoRow>
        <InfoRow label={t('detail.created')}>{volume.CreatedAt ? formatDate(volume.CreatedAt) : '-'}</InfoRow>
      </div>

      <div className="mc-glass rounded-lg p-6">
        <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
          {t('detail.usage')}
        </h3>
        <InfoRow label={t('detail.status')}>
          <StatusIndicator
            tone={mountCount > 0 ? 'success' : 'neutral'}
            label={mountCount > 0 ? t('badges.inUse') : t('badges.unused')}
            size="sm"
          />
        </InfoRow>
        <InfoRow label={t('detail.mountedBy')}>{mountCount}</InfoRow>
        <InfoRow label={t('detail.size')}>{size !== undefined ? formatBytes(size) : t('detail.sizeUnknown')}</InfoRow>
        <p className="mt-4 text-xs text-muted-foreground">{t('detail.sizeDescription')}</p>
      </div>
    </div>
  );
}
