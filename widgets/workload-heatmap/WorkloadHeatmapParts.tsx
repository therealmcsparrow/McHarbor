// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import type { ContainerInfo } from '@core/types/docker';
import { TONE_SOLID, type Tone } from '@modules/dashboard/components/widget-kit';
import { Popover, PopoverAnchor, PopoverContent } from '@resources/components/ui/Popover';
import { cn } from '@resources/utils/cn';
import { containerName } from './WorkloadCellCard';

/**
 * The name label that follows the pointer across the grid.
 *
 * A second Popover root rather than part of the details card: it must never
 * take focus, never trap the pointer, and disappear the instant a card opens.
 * Like the card it anchors through a `virtualRef`, so the whole grid costs one
 * root instead of one per cell.
 */
export function WorkloadHoverLabel({
  container,
  anchorRef,
  tone,
  open: isOpen,
}: {
  container: ContainerInfo | null;
  anchorRef: RefObject<HTMLButtonElement | null>;
  tone: Tone;
  open: boolean;
}) {
  const { t } = useTranslation('dashboard');

  return (
    <Popover open={isOpen}>
      <PopoverAnchor virtualRef={anchorRef} />
      <PopoverContent
        align="center"
        side="top"
        sideOffset={6}
        collisionPadding={12}
        onOpenAutoFocus={(evt) => evt.preventDefault()}
        className="pointer-events-none flex w-auto max-w-[16rem] items-center gap-2 px-2.5 py-1.5"
      >
        {container && (
          <>
            <span className={cn('size-2 shrink-0 rounded-full', TONE_SOLID[tone])} aria-hidden="true" />
            <span className="truncate text-xs font-medium text-foreground">{containerName(container)}</span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {t(`workloadHeatmapWidget.state.${container.State}`)}
            </span>
          </>
        )}
      </PopoverContent>
    </Popover>
  );
}

/** The per-state tally under the grid, in the same order the cells are grouped. */
export function WorkloadLegend({ entries }: { entries: Array<{ state: string; count: number; tone: Tone }> }) {
  const { t } = useTranslation('dashboard');

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
      {entries.map((entry) => (
        <span key={entry.state} className="inline-flex items-center gap-1.5">
          <span className={cn('size-2 rounded-full', TONE_SOLID[entry.tone])} aria-hidden="true" />
          <span>{t(`workloadHeatmapWidget.state.${entry.state}`)}</span>
          <span className="text-foreground tabular-nums">{entry.count}</span>
        </span>
      ))}
    </div>
  );
}
