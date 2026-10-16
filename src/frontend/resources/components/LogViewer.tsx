// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { detectLogLevel, logSeverityClass } from '@resources/utils/log-severity';

type LogViewerProps = {
  lines: string[];
  emptyMessage?: string;
  autoScroll?: boolean;
  className?: string;
  onLineSelect?: (line: string, index: number) => void;
  getLineAriaLabel?: (line: string, index: number) => string;
  lineIds?: string[];
  levels?: string[];
  colorizeSeverity?: boolean;
};

function handleLineKeyDown(
  evt: ReactKeyboardEvent<HTMLButtonElement>,
  index: number,
  lineIds: string[],
  setActiveLineId: (lineId: string) => void,
) {
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(evt.key)) return;
  const buttons = evt.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[data-log-line]');
  if (!buttons?.length) return;
  evt.preventDefault();
  const nextIndex =
    evt.key === 'Home'
      ? 0
      : evt.key === 'End'
        ? buttons.length - 1
        : evt.key === 'ArrowDown'
          ? Math.min(index + 1, buttons.length - 1)
          : Math.max(index - 1, 0);
  setActiveLineId(lineIds[nextIndex] ?? lineIds[0] ?? '');
  buttons[nextIndex]?.focus();
}

export function LogViewer({
  lines,
  emptyMessage,
  autoScroll = true,
  className,
  onLineSelect,
  getLineAriaLabel,
  lineIds,
  levels,
  colorizeSeverity = false,
}: LogViewerProps) {
  const ref = useRef<HTMLDivElement>(null);
  const resolvedLineIds = lines.map((line, index) => lineIds?.[index] ?? `log-line-${index + 1}-${line}`);
  const severityClasses = lines.map((line, index) => {
    if (levels) return logSeverityClass(levels[index]);
    return colorizeSeverity ? logSeverityClass(detectLogLevel(line)) : '';
  });
  const [activeLineId, setActiveLineId] = useState('');
  const effectiveActiveLineId = resolvedLineIds.includes(activeLineId) ? activeLineId : (resolvedLineIds[0] ?? '');

  useEffect(() => {
    if (autoScroll && ref.current) {
      ref.current.scrollTop = ref.current.scrollHeight;
    }
  }, [lines, autoScroll]);

  return (
    <div className={cn('min-h-[300px] overflow-hidden rounded-lg border border-border bg-background', className)}>
      <div
        ref={ref}
        role="log"
        aria-live="off"
        data-scroll-exempt
        className="h-full overflow-auto p-4 font-mono text-xs leading-5 text-foreground"
      >
        {lines.map((line, i) =>
          onLineSelect ? (
            <Button
              key={resolvedLineIds[i]}
              variant="ghost"
              onClick={() => onLineSelect(line, i)}
              onKeyDown={(evt) => handleLineKeyDown(evt, i, resolvedLineIds, setActiveLineId)}
              aria-label={getLineAriaLabel?.(line, i)}
              aria-haspopup="dialog"
              data-log-line
              tabIndex={resolvedLineIds[i] === effectiveActiveLineId ? 0 : -1}
              className={cn(
                'block h-auto min-h-11 w-full justify-start rounded-none px-0 py-0 text-left font-mono text-xs leading-5 whitespace-pre-wrap sm:min-h-5',
                severityClasses[i],
              )}
            >
              {line || ' '}
            </Button>
          ) : (
            <div
              key={`log-line-${i + 1}-${line}`}
              className={cn('min-h-5 whitespace-pre-wrap hover:bg-muted/30', severityClasses[i])}
            >
              {line}
            </div>
          ),
        )}
        {lines.length === 0 && emptyMessage && <span className="text-muted-foreground">{emptyMessage}</span>}
      </div>
    </div>
  );
}
