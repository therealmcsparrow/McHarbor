// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCircleCheck, IconCircleX } from '@tabler/icons-react';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { Spinner } from '@resources/components/ui/Spinner';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import type { MigrationPhase, MigrationStatus } from '../types';

type DatabaseMigrationOutcomeProps = {
  phase: MigrationPhase | 'queued';
  status: MigrationStatus | null | undefined;
  percent: number;
};

/**
 * The one panel under the phase chips that explains where the job stands:
 * failed, cancelled, finished, or still copying.
 *
 * Split out of DatabaseMigrationDialog because it is four mutually exclusive
 * renderings of the same slot, and reading them next to the dialog's chrome
 * made neither half easy to follow.
 */
export function DatabaseMigrationOutcome({ phase, status: currentStatus, percent }: DatabaseMigrationOutcomeProps) {
  const { t } = useTranslation('database');

  if (phase === 'failed') {
    return (
      <Flex align="start" gap={3} className="rounded-lg border border-destructive/40 bg-destructive/5 p-3">
        <IconCircleX className="mt-0.5 size-5 shrink-0 text-destructive" aria-hidden />
        <Stack gap={1}>
          <TypographyText weight="medium" tone="destructive">
            {t('migration.failed')}
          </TypographyText>
          {currentStatus?.error && (
            <TypographyText size="sm">
              {/* eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: error codes come from the backend and fall back to the raw code or an unknown label */}
              {t(`errors.${currentStatus.error}`, { defaultValue: currentStatus.error })}
            </TypographyText>
          )}
          {currentStatus?.errorDetail && (
            <TypographyText size="xs" tone="muted" className="font-mono break-all">
              {currentStatus.errorDetail}
            </TypographyText>
          )}
          <TypographyText size="sm" tone="muted">
            {t('migration.failedSafe')}
          </TypographyText>
        </Stack>
      </Flex>
    );
  }

  if (phase === 'cancelled') {
    return (
      <Flex align="start" gap={3} className="rounded-lg border border-warning/40 bg-warning/5 p-3">
        <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-warning" aria-hidden />
        <Stack gap={1}>
          <TypographyText weight="medium">{t('migration.cancelled')}</TypographyText>
          <TypographyText size="sm" tone="muted">
            {t('migration.failedSafe')}
          </TypographyText>
        </Stack>
      </Flex>
    );
  }

  if (phase === 'done') {
    return (
      <Flex align="start" gap={3} className="rounded-lg border border-success/40 bg-success/5 p-3">
        <IconCircleCheck className="mt-0.5 size-5 shrink-0 text-success" aria-hidden />
        <Stack gap={1}>
          <TypographyText weight="medium">{t('migration.done')}</TypographyText>
          {Boolean(currentStatus?.rowsCaughtUp) && (
            <TypographyText size="sm" tone="muted">
              {t('migration.caughtUp', { count: currentStatus?.rowsCaughtUp ?? 0 })}
            </TypographyText>
          )}
          <TypographyText size="sm" tone="muted">
            {currentStatus?.restartScheduled ? t('migration.restartScheduled') : t('migration.restartRequired')}
          </TypographyText>
        </Stack>
      </Flex>
    );
  }

  return (
    <Stack gap={2}>
      <Flex align="center" justify="between" gap={2} wrap>
        <Flex align="center" gap={2}>
          <Spinner className="size-4" />
          <TypographyText size="sm">{t(`migration.phase.${phase}`)}</TypographyText>
        </Flex>
        <TypographyText size="sm" tone="muted">
          {currentStatus
            ? t('migration.rows', {
                copied: new Intl.NumberFormat().format(currentStatus.rowsCopied),
                total: new Intl.NumberFormat().format(currentStatus.rowsTotal),
              })
            : ''}
        </TypographyText>
      </Flex>
      <ProgressBar value={percent} />
      {phase === 'catchup' ? (
        <TypographyText size="sm" tone="muted">
          {t('migration.deltaPass', { pass: currentStatus?.deltaPass ?? 1 })}
        </TypographyText>
      ) : (
        currentStatus?.currentTable && (
          <TypographyText size="sm" tone="muted">
            {t('migration.currentTable', {
              table: currentStatus.currentTable,
              index: currentStatus.tableIndex,
              total: currentStatus.tableTotal,
            })}
          </TypographyText>
        )
      )}
    </Stack>
  );
}
