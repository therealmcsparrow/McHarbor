// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Subheading, Text as BodyText } from '@resources/components/ui/Typography';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { STATUS_CARD, STATUS_CARD_BODY, STATUS_CARD_SUMMARY } from '@resources/components/ui/status-card';
import { StatusStrip, type StripBlock, type StripTone } from '@resources/components/ui/StatusStrip';
import { formatTime } from '@resources/utils/format';
import type { AgentHealthHistory, AgentHealthSample } from '@core/types/docker';
import { AGENT_HEALTH_HISTORY_LIMIT } from '../hooks/useAgentHealthHistory';

type AgentUptimeCardProps = {
  history?: AgentHealthHistory;
  loading: boolean;
};

/**
 * The Agent uptime card. Same visual language as its container counterpart,
 * but each block is one sampled observation of whether the agent's outbound
 * WebSocket was up.
 *
 * Uptime percentage and outage count are shown together because neither is
 * sufficient alone: an agent that dropped once for an hour and one that flapped
 * sixty times produce the same percentage, and only the second is a networking
 * problem worth chasing.
 */
export function AgentUptimeCard({ history: agentHistory, loading }: AgentUptimeCardProps) {
  const { t } = useTranslation('environments');

  const blocks: StripBlock[] = (agentHistory?.samples ?? []).map((sample) => ({
    key: sample.sampledAt,
    tone: toneFor(sample),
    label: t('detail.agentUptime.sample', {
      time: formatTime(sample.sampledAt),
      state: sample.connected ? t('detail.agentUptime.connected') : t('detail.agentUptime.disconnected'),
    }),
  }));

  const percent = agentHistory ? Math.round(agentHistory.uptimePercent * 10) / 10 : 0;

  return (
    <Box className={STATUS_CARD}>
      <Flex align="center" justify="between" gap={3} className="mb-4">
        <Subheading className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
          {t('detail.agentUptime.title')}
        </Subheading>
        {agentHistory && agentHistory.samples.length > 0 && (
          <Badge variant={agentHistory.downCount === 0 ? 'success' : 'warning'}>
            {t('detail.agentUptime.percent', { percent })}
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
          {/* Takes the slack so the strip and footer below sit at the
              bottom of the card, level with its neighbour. */}
          <BodyText tone="muted" size="sm" className={STATUS_CARD_SUMMARY}>
            {summary(t, agentHistory, percent)}
          </BodyText>

          <StatusStrip
            blocks={blocks}
            slots={AGENT_HEALTH_HISTORY_LIMIT}
            ariaLabel={t('detail.agentUptime.stripLabel', {
              total: blocks.length,
              up: agentHistory?.connectedCount ?? 0,
              down: agentHistory?.downCount ?? 0,
            })}
          />

          <Flex align="center" justify="between" gap={3} className="mt-3 flex-wrap">
            <BodyText tone="muted" size="xs">
              {t('detail.agentUptime.footer', {
                count: blocks.length,
                interval: agentHistory?.sampleIntervalSeconds ?? 30,
              })}
            </BodyText>
            <BodyText tone="muted" size="xs">
              {t('detail.agentUptime.counts', {
                up: agentHistory?.connectedCount ?? 0,
                down: agentHistory?.downCount ?? 0,
                outages: agentHistory?.outages ?? 0,
              })}
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
  return 'pass';
}

/** Builds the sentence above the strip. */
function summary(
  t: (key: string, options?: Record<string, unknown>) => string,
  agentHistory: AgentHealthHistory | undefined,
  percent: number,
): string {
  if (!agentHistory || agentHistory.samples.length === 0) {
    return t('detail.agentUptime.summaryNoSamples');
  }
  if (agentHistory.downCount === 0) {
    return t('detail.agentUptime.summaryAllUp', { total: agentHistory.samples.length });
  }
  return t('detail.agentUptime.summaryWithDowntime', {
    down: agentHistory.downCount,
    total: agentHistory.samples.length,
    percent,
    outages: agentHistory.outages,
  });
}
