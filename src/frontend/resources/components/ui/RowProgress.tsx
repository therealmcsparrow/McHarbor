// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconAlertCircle, IconCircleCheckFilled, IconClock, IconLoader2 } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Caption } from '@resources/components/ui/Typography';
import { Flex, Box } from '@resources/components/ui/Layout';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { cn } from '@resources/utils/cn';

export type RowProgressStatus = 'idle' | 'running' | 'success' | 'error' | 'skipped';

export type RowProgressProps = {
  status: RowProgressStatus;
  /** Progress numerator. Ignored unless status === 'running' and total > 0. */
  done?: number;
  /** Progress denominator. The bar is suppressed when total <= 0. */
  total?: number;
  /** Optional override for the "X / Y" caption. */
  label?: string;
  /**
   * - `cell`: status icon + slim bar + caption, sized for a table cell.
   * - `inline`: status icon + slim bar + caption, full-width row.
   * - `compact`: just the slim bar + numeric caption, no icon.
   */
  variant?: 'cell' | 'inline' | 'compact';
  className?: string;
};

/**
 * `<RowProgress>` is the canonical surface for in-flight, per-row
 * progress. It renders the same status icon set as the dialog row
 * (running / success / error / skipped / idle) plus a thin progress bar
 * driven by `done` / `total`.
 *
 * Designed to drop into any `ColumnDef.cell` of a `DataGrid` row when
 * the underlying table needs to communicate that a row is mid-mutation
 * after a batch operation completes. When `total <= 0` (the producer
 * doesn't know how many steps remain) the bar is suppressed and only
 * the running icon is rendered, falling back to an indeterminate
 * spinner cue.
 */
export function RowProgress({
  status: currentStatus,
  done = 0,
  total = 0,
  label,
  variant = 'cell',
  className,
}: RowProgressProps) {
  const { t } = useTranslation('common');

  const icon = getStatusIcon(currentStatus);
  const showBar = currentStatus === 'running' && total > 0;
  const caption =
    label ?? (currentStatus === 'running' && total > 0 ? t('operations.itemProgress', { done, total }) : null);

  if (variant === 'compact') {
    return (
      <Flex direction="col" gap={1} className={cn('min-w-[120px]', className)}>
        {showBar ? (
          <ProgressBar value={done} max={total} size="sm" />
        ) : (
          <Box className="h-1.5 w-full rounded-full bg-muted" aria-hidden="true" />
        )}
        <Caption size="xs" tone="muted" className="tabular-nums">
          {caption ?? t(`operations.itemStatus.${currentStatus}`)}
        </Caption>
      </Flex>
    );
  }

  const isCell = variant === 'cell';

  return (
    <Flex
      align="center"
      gap={isCell ? 2 : 3}
      className={cn(isCell ? 'min-w-[160px]' : 'min-w-[200px]', className)}
      role={currentStatus === 'running' ? 'status' : undefined}
      aria-live={currentStatus === 'running' ? 'polite' : undefined}
    >
      {icon}
      <Box className="min-w-0 flex-1">
        {showBar && <ProgressBar value={done} max={total} size="sm" />}
        {caption && (
          <Caption size="xs" tone="muted" className="tabular-nums">
            {caption}
          </Caption>
        )}
      </Box>
    </Flex>
  );
}

function getStatusIcon(currentStatus: RowProgressStatus) {
  if (currentStatus === 'success') {
    return <IconCircleCheckFilled className="size-4 shrink-0 text-success" aria-hidden="true" />;
  }
  if (currentStatus === 'error') {
    return <IconAlertCircle className="size-4 shrink-0 text-warning" aria-hidden="true" />;
  }
  if (currentStatus === 'skipped') {
    return <IconClock className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />;
  }
  if (currentStatus === 'running') {
    return <IconLoader2 className="size-4 shrink-0 animate-spin text-primary" aria-hidden="true" />;
  }
  return <IconClock className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />;
}
