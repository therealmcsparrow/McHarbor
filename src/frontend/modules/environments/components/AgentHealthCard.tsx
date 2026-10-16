// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Subheading, Text as BodyText } from '@resources/components/ui/Typography';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { STATUS_CARD, STATUS_CARD_BODY, STATUS_CARD_FILL } from '@resources/components/ui/status-card';
import { StatusStrip, type StripBlock, type StripTone } from '@resources/components/ui/StatusStrip';
import { CHART_COLORS } from '@resources/theme/chart-colors';
import { formatTime } from '@resources/utils/format';
import type { AgentHealth, AgentHealthHistory, AgentHealthSample } from '@core/types/docker';
import { AGENT_HEALTH_HISTORY_LIMIT } from '../hooks/useAgentHealthHistory';

type AgentHealthCardProps = {
  history?: AgentHealthHistory;
  loading: boolean;
};

/**
 * The Agent health card. Each block is one sampled observation of the
 * keepalive, not of the socket: a full block means the agent answered a ping
 * promptly, a half-height block means it is connected but degraded — a stale
 * pong or a round trip slow enough that ordinary Docker calls are already into
 * timeout territory — and a squat bar means there was no connection to probe.
 *
 * The chart underneath is the ping/pong round trip. It is the part that
 * actually diagnoses a bad link: a strip that is entirely green while the
 * latency line climbs is the shape of an agent that is about to start timing
 * out, and nothing in the old connected/disconnected badge could show it.
 */
export function AgentHealthCard({ history: agentHistory, loading }: AgentHealthCardProps) {
  const { t } = useTranslation('environments');

  const blocks: StripBlock[] = (agentHistory?.samples ?? []).map((sample) => ({
    key: sample.sampledAt,
    tone: toneFor(sample),
    label: t('detail.agentHealth.sample', {
      time: formatTime(sample.sampledAt),
      health: t(`detail.agentHealth.state.${sample.health}`),
    }),
  }));

  const latencyData = agentHistory?.latencySeries ?? [];

  return (
    <Box className={STATUS_CARD}>
      <Flex align="center" justify="between" gap={3} className="mb-4">
        <Subheading className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
          {t('detail.agentHealth.title')}
        </Subheading>
        {agentHistory && (
          <Badge variant={badgeVariantFor(agentHistory.health)}>
            {t(`detail.agentHealth.state.${agentHistory.health}`)}
          </Badge>
        )}
      </Flex>

      {loading && !agentHistory ? (
        <Box className="space-y-3">
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-7 w-full" />
          <Skeleton className="h-3 w-1/3" />
        </Box>
      ) : (
        <Box className={STATUS_CARD_BODY}>
          <BodyText tone="muted" size="sm" className="mb-4">
            {summary(t, agentHistory)}
          </BodyText>

          {/* The chart sits above the strip and takes the slack, so this card
              ends with the same strip-then-footer pair as the other three and
              lines up with them instead of standing a chart taller. */}
          <Box className={STATUS_CARD_FILL}>
            {latencyData.length > 1 && (
              <StatsAreaChart
                data={latencyData}
                dataKey="value"
                xAxisKey="timestamp"
                color={CHART_COLORS.networkRx}
                formatValue={(value) => t('detail.agentHealth.ms', { value: Math.round(value) })}
                label={t('detail.agentHealth.latency')}
                // compact drops the chart's own 120px floor and lets it fill
                // whatever the card has spare, so it cannot push this card
                // taller than its neighbour.
                compact
              />
            )}
          </Box>

          <StatusStrip
            blocks={blocks}
            slots={AGENT_HEALTH_HISTORY_LIMIT}
            ariaLabel={t('detail.agentHealth.stripLabel', {
              total: blocks.length,
              degraded: agentHistory?.degradedCount ?? 0,
              down: agentHistory?.downCount ?? 0,
            })}
          />

          <Flex align="center" justify="between" gap={3} className="mt-3 flex-wrap">
            <BodyText tone="muted" size="xs">
              {t('detail.agentHealth.footer', {
                count: blocks.length,
                interval: agentHistory?.sampleIntervalSeconds ?? 30,
              })}
            </BodyText>
            <BodyText tone="muted" size="xs">
              {latencyData.length > 0
                ? t('detail.agentHealth.latencyCounts', {
                    avg: agentHistory?.avgLatencyMs ?? 0,
                    max: agentHistory?.maxLatencyMs ?? 0,
                  })
                : t('detail.agentHealth.latencyUnavailable')}
            </BodyText>
          </Flex>
        </Box>
      )}
    </Box>
  );
}

function toneFor(sample: AgentHealthSample): StripTone {
  if (!sample.connected) return 'fail';
  if (sample.health === 'degraded') return 'warn';
  if (sample.health === 'unknown') return 'idle';
  return 'pass';
}

function badgeVariantFor(health: AgentHealth) {
  switch (health) {
    case 'healthy':
      return 'success' as const;
    case 'degraded':
      return 'warning' as const;
    case 'unhealthy':
      return 'destructive' as const;
    default:
      return 'secondary' as const;
  }
}

/** Builds the sentence above the strip. */
function summary(
  t: (key: string, options?: Record<string, unknown>) => string,
  agentHistory?: AgentHealthHistory,
): string {
  if (!agentHistory || agentHistory.samples.length === 0) {
    return t('detail.agentHealth.summaryNoSamples');
  }
  if (agentHistory.health === 'unknown') {
    return t('detail.agentHealth.summaryNeverConnected');
  }
  if (agentHistory.health === 'unhealthy') {
    return t('detail.agentHealth.summaryDisconnected', {
      time: agentHistory.lastSeen ? formatTime(agentHistory.lastSeen) : '',
    });
  }
  if (agentHistory.degradedCount > 0) {
    return t('detail.agentHealth.summaryDegraded', {
      degraded: agentHistory.degradedCount,
      total: agentHistory.samples.length,
      interval: agentHistory.pingIntervalSeconds,
    });
  }
  return t('detail.agentHealth.summaryHealthy', {
    total: agentHistory.samples.length,
    interval: agentHistory.pingIntervalSeconds,
    latency: agentHistory.latencyMs ?? agentHistory.avgLatencyMs,
  });
}
