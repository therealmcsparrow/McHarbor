// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type DragEvent as ReactDragEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowDown, IconArrowUp, IconGripVertical, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { TableEditColumnSettings } from './TableEditColumnSettings';
import type { TableColumnAlign, TableColumnWidth } from './types';

const DRAG_TYPE = 'table/column';

type TableEditShownListProps = {
  ids: string[];
  labels: Record<string, string>;
  sizes: Record<string, TableColumnWidth>;
  headerAlign: Record<string, TableColumnAlign>;
  cellAlign: Record<string, TableColumnAlign>;
  sortable: Record<string, boolean>;
  sortCapable: Set<string>;
  locked: Set<string>;
  onMove: (id: string, delta: -1 | 1) => void;
  onMoveTo: (id: string, targetId: string) => void;
  onHide: (id: string) => void;
  onResize: (id: string, size: TableColumnWidth) => void;
  onAlign: (id: string, part: 'header' | 'cell', value: TableColumnAlign) => void;
  onSortable: (id: string, value: boolean) => void;
};

export function TableEditShownList({
  ids,
  labels,
  sizes,
  headerAlign,
  cellAlign,
  sortable,
  sortCapable,
  locked,
  onMove,
  onMoveTo,
  onHide,
  onResize,
  onAlign,
  onSortable,
}: TableEditShownListProps) {
  const { t } = useTranslation('common');
  const [over, setOver] = useState<string | null>(null);

  const onDragStart = (evt: ReactDragEvent<HTMLElement>, id: string) => {
    evt.dataTransfer.setData(DRAG_TYPE, id);
    evt.dataTransfer.effectAllowed = 'move';
  };

  const onDrop = (evt: ReactDragEvent<HTMLElement>, targetId: string) => {
    evt.preventDefault();
    setOver(null);
    const id = evt.dataTransfer.getData(DRAG_TYPE);
    if (id) onMoveTo(id, targetId);
  };

  return (
    <ul className="flex flex-col gap-1" aria-label={t('tableViews.shown')}>
      {ids.map((id, index) => {
        const isLocked = locked.has(id);
        return (
          <li key={id}>
            <div
              role="presentation"
              draggable={!isLocked}
              onDragStart={(evt) => onDragStart(evt, id)}
              onDragOver={(evt) => {
                if (isLocked || !evt.dataTransfer.types.includes(DRAG_TYPE)) return;
                evt.preventDefault();
                setOver(id);
              }}
              onDragLeave={() => setOver((current) => (current === id ? null : current))}
              onDrop={(evt) => onDrop(evt, id)}
              className={cn(
                'flex items-center gap-2 rounded-md border border-border bg-card px-2 py-1.5',
                over === id && 'border-primary',
              )}
            >
              <IconGripVertical
                className={cn('size-4 text-muted-foreground', isLocked ? 'invisible' : 'cursor-grab')}
              />
              <span className={cn('min-w-0 flex-1 truncate text-sm text-foreground', isLocked && 'opacity-70')}>
                {labels[id] ?? id}
              </span>
              <TableEditColumnSettings
                id={id}
                label={labels[id] ?? id}
                size={sizes[id] ?? 'auto'}
                headerAlign={headerAlign[id] ?? 'auto'}
                cellAlign={cellAlign[id] ?? 'auto'}
                onResize={onResize}
                onAlign={onAlign}
                sortable={sortCapable.has(id) ? sortable[id] === true : undefined}
                onSortable={onSortable}
              />
              {!isLocked && (
                <>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    disabled={index === 0}
                    aria-label={t('tableViews.moveUp', { column: labels[id] ?? id })}
                    onClick={() => onMove(id, -1)}
                  >
                    <IconArrowUp className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    disabled={index === ids.length - 1}
                    aria-label={t('tableViews.moveDown', { column: labels[id] ?? id })}
                    onClick={() => onMove(id, 1)}
                  >
                    <IconArrowDown className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('tableViews.removeColumn', { column: labels[id] ?? id })}
                    onClick={() => onHide(id)}
                  >
                    <IconX className="size-4" />
                  </Button>
                </>
              )}
            </div>
          </li>
        );
      })}
    </ul>
  );
}
