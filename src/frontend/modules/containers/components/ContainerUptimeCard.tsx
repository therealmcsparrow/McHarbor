// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Subheading, Text as BodyText } from '@resources/components/ui/Typography';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { formatTime } from '@resources/utils/format';
import { STATUS_CARD, STATUS_CARD_BODY, STATUS_CARD_SUMMARY } from '@resources/components/ui/status-card';
import { StatusStrip, type StripBlock } from '@resources/components/ui/StatusStrip';
import { HEALTH_HISTORY_LIMIT } from '../hooks/useContainerHealthHistory';
import type { ContainerHealthHistory, UptimeSample } from '@core/types/docker';

type ContainerUptimeCardProps = {
  history?: ContainerHealthHistory;
  loading: boolean;
};

/**
 * The Uptime card. Same visual language as the Health card, but each block is
 * one sampled observation of the container's run state rather than a probe:
 * a full block means it was running, a squat bar means it was not, and a
 * half-height block means it was running but reporting unhealthy.
 */
export function ContainerUptimeCard({ history: historyPoints, loading }: ContainerUptimeCardProps) {
  const { t } = useTranslation('containers');

  const blocks: StripBlock[] = (historyPoints?.samples ?? []).map((sample) => ({
    key: sample.sampledAt,
    tone: toneFor(sample),
    label: t('uptimeCard.sample', {
      time: formatTime(sample.sampledAt),
      state: sample.state,
    }),
  }));

  const percent = historyPoints ? Math.round(historyPoints.uptimePercent * 10) / 10 : 0;

  return (
    <Box className={STATUS_CARD}>
      <Flex align="center" justify="between" gap={3} className="mb-4">
        <Subheading className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
          {t('uptimeCard.title')}
        </Subheading>
        {historyPoints && historyPoints.samples.length > 0 && (
          <Badge variant={historyPoints.downCount === 0 ? 'success' : 'warning'}>
            {t('uptimeCard.percent', { percent })}
          </Badge>
        )}
      </Flex>

      {loading && !historyPoints ? (
        <Box className="space-y-3">
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-7 w-full" />
          <Skeleton className="h-3 w-1/3" />
        </Box>
      ) : (
        <Box className={STATUS_CARD_BODY}>
          {/* Takes the slack so the strip and footer below sit at the
              bottom of the card, level with its neighbour. */}
          <BodyText tone="muted" size="sm" className={STATUS_CARD_SUMMARY}>
            {summary(t, historyPoints, percent)}
          </BodyText>

          <StatusStrip
            blocks={blocks}
            slots={HEALTH_HISTORY_LIMIT}
            ariaLabel={t('uptimeCard.stripLabel', {
              total: blocks.length,
              up: historyPoints?.runningCount ?? 0,
              down: historyPoints?.downCount ?? 0,
            })}
          />

          <Flex align="center" justify="between" gap={3} className="mt-3 flex-wrap">
            <BodyText tone="muted" size="xs">
              {t('uptimeCard.footer', {
                count: blocks.length,
                interval: historyPoints?.sampleIntervalSeconds ?? 30,
              })}
            </BodyText>
            <BodyText tone="muted" size="xs">
              {t('uptimeCard.counts', {
                up: historyPoints?.runningCount ?? 0,
                down: historyPoints?.downCount ?? 0,
              })}
            </BodyText>
          </Flex>
        </Box>
      )}
    </Box>
  );
}

function toneFor(sample: UptimeSample) {
  if (!sample.running) return 'fail' as const;
  if (sample.health === 'unhealthy' || sample.health === 'starting') {
    return 'warn' as const;
  }
  return 'pass' as const;
}

/** Builds the sentence above the strip. */
function summary(
  t: (key: string, options?: Record<string, unknown>) => string,
  historyData: ContainerHealthHistory | undefined,
  percent: number,
): string {
  if (!historyData || historyData.samples.length === 0) {
    return t('uptimeCard.summaryNoSamples');
  }
  if (historyData.downCount === 0) {
    return t('uptimeCard.summaryAllUp', {
      total: historyData.samples.length,
      restarts: historyData.restartCount,
    });
  }
  return t('uptimeCard.summaryWithDowntime', {
    down: historyData.downCount,
    total: historyData.samples.length,
    percent,
    restarts: historyData.restartCount,
  });
}
