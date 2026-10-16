// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDatabase, IconNetwork, IconPhoto } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card } from '@resources/components/ui/Card';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { StatusError } from '@resources/components/ui/StatusError';
import { formatDate } from '@resources/utils/format';
import { formatBytes } from '../../format';
import { PoolCapacityBar } from '../PoolCapacityBar';
import { StatTile } from '../StatTile';
import type { OverviewEntry } from '../../types';

interface HostOverviewTabProps {
  entry: OverviewEntry;
  onNavigate: (to: string) => void;
}

export function HostOverviewTab({ entry, onNavigate }: HostOverviewTabProps) {
  const { t } = useTranslation('compute');
  const pools = entry.pools ?? [];
  const networks = entry.networks ?? [];
  const capacity = pools.reduce((sum, pool) => sum + pool.capacityBytes, 0);
  const used = pools.reduce((sum, pool) => sum + pool.usedBytes, 0);

  return (
    <div className="space-y-4">
      <Card className="p-5">
        <dl className="grid gap-x-8 gap-y-3 sm:grid-cols-2">
          <InfoRow label={t('hosts.overview.os')}>{entry.capability?.os || t('capability.neverProbed')}</InfoRow>
          <InfoRow label={t('hosts.overview.arch')}>{entry.capability?.arch || t('capability.neverProbed')}</InfoRow>
          <InfoRow label={t('hosts.overview.kernel')}>
            {entry.capability?.kernelVersion || t('hosts.overview.unknown')}
          </InfoRow>
          <InfoRow label={t('hosts.overview.agentVersion')}>
            {entry.capability?.agentVersion || t('hosts.overview.unknown')}
          </InfoRow>
          <InfoRow label={t('hosts.overview.probedAt')}>
            {entry.capability?.probedAt ? formatDate(entry.capability.probedAt) : t('capability.neverProbed')}
          </InfoRow>
          <InfoRow label={t('overview.sharedLibrary')}>
            {entry.sharedLibrary ? t('overview.sharedLibraryOn') : t('overview.sharedLibraryOff')}
          </InfoRow>
        </dl>
        {entry.capability?.unsupportedWhy && (
          <div className="mt-4 flex items-center gap-2">
            <span className="text-sm text-muted-foreground">{t('hosts.overview.unsupported')}</span>
            <StatusError error={entry.capability.unsupportedWhy} />
          </div>
        )}
      </Card>

      <div className="grid gap-4 lg:grid-cols-3">
        <StatTile
          label={t('nav.pools')}
          value={pools.length > 0 ? `${formatBytes(used)} / ${formatBytes(capacity)}` : t('overview.needsPool')}
          tone={pools.length > 0 ? 'default' : 'warning'}
          icon={<IconDatabase className="size-3.5" aria-hidden="true" />}
        />
        <StatTile
          label={t('nav.networks')}
          value={networks.length > 0 ? networks.map((network) => network.name).join(', ') : t('overview.needsNetwork')}
          tone={networks.length > 0 ? 'default' : 'warning'}
          icon={<IconNetwork className="size-3.5" aria-hidden="true" />}
        />
        <StatTile
          label={t('nav.images')}
          value={entry.cachedImages}
          icon={<IconPhoto className="size-3.5" aria-hidden="true" />}
        />
      </div>

      {pools.length > 0 && (
        <Card className="space-y-4 p-5">
          <h2 className="text-sm font-medium text-foreground">{t('nav.pools')}</h2>
          {pools.map((pool) => (
            <div key={pool.id} className="space-y-1">
              <PoolCapacityBar pool={pool} />
              <div className="flex items-center gap-2">
                <p className="truncate font-mono text-xs text-muted-foreground">{pool.target}</p>
                <StatusError error={pool.lastError} />
              </div>
            </div>
          ))}
        </Card>
      )}

      <div className="flex flex-wrap gap-2">
        <Button variant="outline" size="sm" onClick={() => onNavigate('/pools')}>
          {t('nav.pools')}
        </Button>
        <Button variant="outline" size="sm" onClick={() => onNavigate('/networks')}>
          {t('nav.networks')}
        </Button>
        <Button variant="outline" size="sm" onClick={() => onNavigate('/images')}>
          {t('nav.images')}
        </Button>
      </div>
    </div>
  );
}
