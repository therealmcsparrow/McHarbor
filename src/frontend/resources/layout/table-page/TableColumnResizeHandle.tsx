// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { nearestWidth } from './table-layout';
import { TABLE_COLUMN_WIDTHS, type TableColumnWidth } from './types';

type TableColumnResizeHandleProps = {
  label: string;
  size: TableColumnWidth;
  onCommit: (size: TableColumnWidth) => void;
};

export function TableColumnResizeHandle({ label, size, onCommit }: TableColumnResizeHandleProps) {
  const { t } = useTranslation('common');
  const start = useRef<{ x: number; width: number } | null>(null);
  const [preview, setPreview] = useState<TableColumnWidth | null>(null);

  const onPointerDown = (evt: ReactPointerEvent<HTMLDivElement>) => {
    evt.stopPropagation();
    const cell = evt.currentTarget.closest('th');
    if (!cell) return;
    start.current = { x: evt.clientX, width: cell.getBoundingClientRect().width };
    evt.currentTarget.setPointerCapture(evt.pointerId);
  };

  const onPointerMove = (evt: ReactPointerEvent<HTMLDivElement>) => {
    if (!start.current) return;
    setPreview(nearestWidth(start.current.width + evt.clientX - start.current.x));
  };

  const onPointerUp = (evt: ReactPointerEvent<HTMLDivElement>) => {
    evt.currentTarget.releasePointerCapture(evt.pointerId);
    start.current = null;
    if (preview && preview !== size) onCommit(preview);
    setPreview(null);
  };

  const onKeyDown = (evt: ReactKeyboardEvent<HTMLDivElement>) => {
    if (evt.key !== 'ArrowLeft' && evt.key !== 'ArrowRight') return;
    evt.preventDefault();
    const current = size === 'auto' ? 3 : TABLE_COLUMN_WIDTHS.indexOf(size);
    const next = Math.max(0, Math.min(TABLE_COLUMN_WIDTHS.length - 1, current + (evt.key === 'ArrowRight' ? 1 : -1)));
    onCommit(TABLE_COLUMN_WIDTHS[next] ?? size);
  };

  const current = preview ?? size;
  const currentIndex = current === 'auto' ? 0 : TABLE_COLUMN_WIDTHS.indexOf(current) + 1;

  return (
    <div
      // eslint-disable-next-line jsx-a11y/prefer-tag-over-role -- a focusable separator is the ARIA window-splitter widget; <hr> is void and non-interactive so it cannot carry tabIndex, key handlers or the preview child
      role="separator"
      aria-orientation="vertical"
      aria-label={t('tableViews.resizeColumn', { column: label })}
      aria-valuenow={currentIndex}
      aria-valuemin={0}
      aria-valuemax={TABLE_COLUMN_WIDTHS.length}
      aria-valuetext={t(`tableViews.size.${preview ?? size}`)}
      tabIndex={0}
      onClick={(evt) => evt.stopPropagation()}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onKeyDown={onKeyDown}
      className={cn(
        'absolute inset-y-0 right-0 w-1.5 cursor-col-resize touch-none select-none hover:bg-primary/40 focus-visible:bg-primary focus-visible:outline-none',
        preview && 'bg-primary',
      )}
    >
      {preview && (
        <span className="pointer-events-none absolute top-full right-2 z-20 mt-1 rounded bg-popover px-1.5 py-0.5 text-xs text-popover-foreground shadow">
          {t(`tableViews.size.${preview}`)}
        </span>
      )}
    </div>
  );
}
