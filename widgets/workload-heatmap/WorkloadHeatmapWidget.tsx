// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconBoxOff } from '@tabler/icons-react';
import type { ContainerInfo, ContainerState } from '@core/types/docker';
import { useContainers } from '@modules/containers/hooks/useContainers';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import {
  TONE_SOLID,
  WidgetBadge,
  WidgetEmpty,
  WidgetFrame,
  WidgetLoading,
  type Tone,
} from '@modules/dashboard/components/widget-kit';
import { Popover, PopoverAnchor, PopoverContent } from '@resources/components/ui/Popover';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { cn } from '@resources/utils/cn';
import { WorkloadCellCard, containerName } from './WorkloadCellCard';
import { WorkloadHoverLabel, WorkloadLegend } from './WorkloadHeatmapParts';

const STATE_TONE: Record<string, Tone> = {
  running: 'success',
  restarting: 'warning',
  paused: 'warning',
  created: 'info',
  removing: 'info',
  exited: 'danger',
  dead: 'critical',
};

function stateTone(state: string): Tone {
  return STATE_TONE[state] ?? 'neutral';
}

/**
 * Cells are grouped in this order, worst last, so the eye lands on the
 * problems: a wall of green with three red squares in the corner is the
 * whole point of the tile.
 */
const STATE_ORDER: ContainerState[] = ['running', 'created', 'removing', 'paused', 'restarting', 'exited', 'dead'];

function stateRank(state: string): number {
  const i = STATE_ORDER.indexOf(state as ContainerState);
  return i === -1 ? STATE_ORDER.length : i;
}

export default function WorkloadHeatmapWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: containers, isLoading } = useContainers(true);

  // One Popover for the whole grid, anchored to whichever cell was clicked.
  // 184 Popover roots would mean 184 context providers for a tile that can
  // only ever show one card.
  const anchorRef = useRef<HTMLButtonElement | null>(null);
  const [selected, setSelected] = useState<ContainerInfo | null>(null);

  // The hover label rides the same one-root/virtual-anchor trick as the
  // details card. It is suppressed while a card is open so the two never
  // stack on top of each other.
  const hoverRef = useRef<HTMLButtonElement | null>(null);
  const [hovered, setHovered] = useState<ContainerInfo | null>(null);

  const { cells, legend, total } = useMemo(() => {
    const list = [...(containers ?? [])].sort((a, b) => {
      const rank = stateRank(a.State) - stateRank(b.State);
      return rank !== 0 ? rank : containerName(a).localeCompare(containerName(b));
    });

    const counts = new Map<string, number>();
    for (const c of list) counts.set(c.State, (counts.get(c.State) ?? 0) + 1);

    return {
      cells: list,
      total: list.length,
      legend: [...counts.entries()]
        .sort(([a], [b]) => stateRank(a) - stateRank(b))
        .map(([state, count]) => ({ state, count, tone: stateTone(state) })),
    };
  }, [containers]);

  function openCell(container: ContainerInfo, element: HTMLButtonElement) {
    anchorRef.current = element;
    setSelected(container);
    setHovered(null);
  }

  function previewCell(container: ContainerInfo, element: HTMLButtonElement) {
    hoverRef.current = element;
    setHovered(container);
  }

  return (
    <WidgetFrame
      title={t('workloadHeatmapWidget.title')}
      badge={total > 0 ? <WidgetBadge>{t('workloadHeatmapWidget.count', { count: total })}</WidgetBadge> : undefined}
      footer={legend.length > 0 ? <WorkloadLegend entries={legend} /> : undefined}
    >
      {isLoading ? (
        <WidgetLoading label={t('loading')} />
      ) : total === 0 ? (
        <WidgetEmpty
          icon={<IconBoxOff className="size-5" aria-hidden="true" />}
          title={t('workloadHeatmapWidget.empty')}
          hint={t('workloadHeatmapWidget.emptyHint')}
        />
      ) : (
        <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-3">
          <Popover
            open={selected !== null}
            onOpenChange={(isOpen) => {
              if (!isOpen) setSelected(null);
            }}
          >
            <PopoverAnchor virtualRef={anchorRef} />

            <fieldset
              className="m-0 grid min-w-0 [grid-template-columns:repeat(auto-fill,minmax(1.375rem,1fr))] gap-1 border-0 p-0"
              aria-label={t('workloadHeatmapWidget.gridLabel', { count: total })}
            >
              {cells.map((container) => {
                const tone = stateTone(container.State);
                const isSelected = selected?.Id === container.Id;
                return (
                  <PlainButton
                    key={container.Id}
                    aria-label={t('workloadHeatmapWidget.cellLabel', {
                      name: containerName(container),
                      state: t(`workloadHeatmapWidget.state.${container.State}`),
                    })}
                    aria-pressed={isSelected}
                    onClick={(evt) => openCell(container, evt.currentTarget)}
                    onMouseEnter={(evt) => previewCell(container, evt.currentTarget)}
                    onMouseLeave={() => setHovered(null)}
                    // Keyboard users arrowing through the grid get the same
                    // name label the pointer does.
                    onFocus={(evt) => previewCell(container, evt.currentTarget)}
                    onBlur={() => setHovered(null)}
                    className={cn(
                      'aspect-square w-full rounded-[3px] opacity-80 transition',
                      'hover:scale-110 hover:opacity-100',
                      TONE_SOLID[tone],
                      isSelected && 'scale-110 opacity-100 ring-2 ring-ring ring-offset-1 ring-offset-card',
                    )}
                  />
                );
              })}
            </fieldset>

            <PopoverContent
              align="center"
              side="top"
              sideOffset={8}
              collisionPadding={12}
              className="w-72"
              // With a virtual anchor there is no trigger to restore focus to,
              // so hand it back to the cell the person actually clicked.
              onCloseAutoFocus={(evt) => {
                evt.preventDefault();
                anchorRef.current?.focus();
              }}
            >
              {selected && (
                <WorkloadCellCard
                  container={selected}
                  tone={stateTone(selected.State)}
                  onOpen={() => {
                    const id = selected.Id;
                    setSelected(null);
                    void navigate(`/containers/${id}`);
                  }}
                />
              )}
            </PopoverContent>
          </Popover>

          <WorkloadHoverLabel
            container={hovered}
            anchorRef={hoverRef}
            tone={hovered ? stateTone(hovered.State) : 'neutral'}
            open={hovered !== null && selected === null}
          />
        </div>
      )}
    </WidgetFrame>
  );
}
