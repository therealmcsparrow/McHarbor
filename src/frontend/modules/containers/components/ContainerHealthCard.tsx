// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAdjustmentsHorizontal } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Subheading, Text as BodyText } from '@resources/components/ui/Typography';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { formatTime } from '@resources/utils/format';
import { STATUS_CARD, STATUS_CARD_BODY, STATUS_CARD_SUMMARY } from '@resources/components/ui/status-card';
import { StatusStrip, type StripBlock } from '@resources/components/ui/StatusStrip';
import { HEALTH_HISTORY_LIMIT } from '../hooks/useContainerHealthHistory';
import type { ContainerHealthHistory } from '@core/types/docker';

type ContainerHealthCardProps = {
  history?: ContainerHealthHistory;
  loading: boolean;
  onTuneProbe: () => void;
};

/**
 * The Health card: a plain-language read of the recent probe series, the
 * probe strip itself, and the counts behind it.
 *
 * The series comes from the backend rather than from `State.Health.Log`
 * because Docker only keeps the last five results — a strip built from the
 * inspect alone would never be more than five blocks long.
 */
export function ContainerHealthCard({ history: historyPoints, loading, onTuneProbe }: ContainerHealthCardProps) {
  const { t } = useTranslation('containers');

  const blocks: StripBlock[] = (historyPoints?.probes ?? []).map((probe) => {
    const passed = probe.exitCode === 0;
    return {
      key: probe.start,
      tone: passed ? 'pass' : 'fail',
      label: passed
        ? t('healthCard.probePassed', {
            time: formatTime(probe.start),
            duration: probe.durationMs,
          })
        : t('healthCard.probeFailed', {
            time: formatTime(probe.start),
            code: probe.exitCode,
          }),
    };
  });

  return (
    <Box className={STATUS_CARD}>
      <Flex align="center" justify="between" gap={3} className="mb-4">
        <Subheading className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
          {t('healthCard.title')}
        </Subheading>
        <Button type="button" variant="outline" size="sm" className="gap-1.5" onClick={onTuneProbe}>
          <IconAdjustmentsHorizontal className="size-4" />
          {t('healthCard.tuneProbe')}
        </Button>
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
            {summary(t, historyPoints)}
          </BodyText>

          <StatusStrip
            blocks={blocks}
            slots={HEALTH_HISTORY_LIMIT}
            ariaLabel={t('healthCard.stripLabel', {
              total: blocks.length,
              pass: historyPoints?.passCount ?? 0,
              fail: historyPoints?.failCount ?? 0,
            })}
          />

          <Flex align="center" justify="between" gap={3} className="mt-3 flex-wrap">
            <BodyText tone="muted" size="xs">
              {historyPoints?.intervalSeconds
                ? t('healthCard.footerWithInterval', {
                    count: blocks.length,
                    interval: historyPoints.intervalSeconds,
                  })
                : t('healthCard.footer', { count: blocks.length })}
            </BodyText>
            <BodyText tone="muted" size="xs">
              {t('healthCard.counts', {
                pass: historyPoints?.passCount ?? 0,
                fail: historyPoints?.failCount ?? 0,
              })}
            </BodyText>
          </Flex>
        </Box>
      )}
    </Box>
  );
}

/** Builds the sentence above the strip. */
function summary(
  t: (key: string, options?: Record<string, unknown>) => string,
  historyData?: ContainerHealthHistory,
): string {
  if (!historyData || !historyData.hasHealthcheck) {
    return t('healthCard.summaryNoHealthcheck');
  }
  if (historyData.probes.length === 0) {
    return t('healthCard.summaryNoProbes');
  }
  if (historyData.failCount === 0) {
    return t('healthCard.summaryAllPassed', {
      total: historyData.probes.length,
      interval: historyData.intervalSeconds,
      timeout: historyData.timeoutSeconds,
    });
  }

  const lastFailure = [...historyData.probes].reverse().find((probe) => probe.exitCode !== 0);

  return t('healthCard.summaryWithFailures', {
    failed: historyData.failCount,
    total: historyData.probes.length,
    time: lastFailure ? formatTime(lastFailure.start) : '',
    code: lastFailure?.exitCode ?? 0,
    timeout: historyData.timeoutSeconds,
  });
}
