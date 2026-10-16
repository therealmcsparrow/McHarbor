// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconChartDots } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useStatsPath, type StatsPath, type StatsPathMode } from '../hooks/useStatsPath';

const modeVariant: Record<StatsPathMode, 'success' | 'warning' | 'secondary' | 'neutral'> = {
  push: 'success',
  pull: 'warning',
  direct: 'neutral',
  off: 'secondary',
};

function useDetail(path: StatsPath): string | null {
  const { t } = useTranslation('environments');
  if (path.reason) {
    return t(`detail.statsPath.reason.${path.reason}`, {
      version: path.agentVersion ?? '-',
      build: path.agentBuild || '-',
      requiredVersion: path.requiredVersion ?? '-',
      requiredBuild: path.requiredBuild ?? '-',
    });
  }
  if (path.mode === 'push' && path.lastBatchAt) {
    return t('detail.statsPath.lastBatch', { time: timeAgo(path.lastBatchAt) });
  }
  return null;
}

export function StatsPathNotice({ envId }: { envId: string }) {
  const { t } = useTranslation('environments');
  const allowed = usePermission(PERM.HOST_VIEW);
  const { data } = useStatsPath(envId, allowed);
  const detail = useDetail(data ?? { mode: 'off' });

  if (!data) return null;

  return (
    <div className="mc-glass rounded-lg p-4 text-sm" data-testid="stats-path">
      <div className="flex flex-wrap items-center gap-2">
        <IconChartDots className="size-4 text-muted-foreground" aria-hidden="true" />
        <span className="text-muted-foreground">{t('detail.statsPath.title')}</span>
        <Badge variant={modeVariant[data.mode]}>
          {t(`detail.statsPath.mode.${data.mode}`, { seconds: data.intervalSeconds ?? 0 })}
        </Badge>
      </div>
      {detail && <p className="mt-2 text-muted-foreground">{detail}</p>}
    </div>
  );
}
