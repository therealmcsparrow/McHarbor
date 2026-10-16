// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cn } from '@resources/utils/cn';
import type { Cell, Column, Status } from '../types';
import { statusPill, statusText } from './tokens';

export function DataTable({
  columns,
  rows,
  caption,
  note,
}: {
  columns: Column[];
  rows: Cell[][];
  caption?: string;
  note?: string;
}) {
  if (columns.length === 0) return null;
  return (
    <div className="mb-6">
      {caption ? (
        <p className="mb-2 text-[11px] font-semibold tracking-wider text-muted-foreground uppercase">{caption}</p>
      ) : null}
      {/* Wide report tables scroll inside their own container so the page
          itself never scrolls sideways. */}
      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full min-w-[560px] border-collapse text-sm">
          <thead>
            <tr className="bg-muted/50">
              {columns.map((c, i) => (
                <th
                  key={`${c.label}-${i}`}
                  scope="col"
                  className={cn(
                    'px-3 py-2 text-[10px] font-semibold tracking-[0.1em] whitespace-nowrap text-muted-foreground uppercase',
                    c.align === 'right' ? 'text-right' : c.align === 'center' ? 'text-center' : 'text-left',
                  )}
                >
                  {c.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, ri) => (
              <tr key={ri} className={cn('border-t border-border', ri % 2 === 1 && 'bg-muted/25')}>
                {columns.map((c, ci) => {
                  const cell = row[ci];
                  return (
                    <td
                      key={ci}
                      className={cn(
                        'px-3 py-2 align-middle',
                        c.align === 'right'
                          ? 'text-right tabular-nums'
                          : c.align === 'center'
                            ? 'text-center'
                            : 'text-left',
                        (cell?.mono || c.mono) && 'font-mono text-xs',
                      )}
                    >
                      <CellView cell={cell} />
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {note ? <p className="mt-2 text-xs text-muted-foreground">{note}</p> : null}
    </div>
  );
}

function CellView({ cell }: { cell?: Cell }) {
  if (!cell || cell.text === '') return <span className="text-muted-foreground">—</span>;
  const statusValue: Status = cell.status ?? '';
  if (cell.pill) {
    return (
      <span className={cn('inline-block rounded-full px-2 py-0.5 text-[11px] font-semibold', statusPill[statusValue])}>
        {cell.text}
      </span>
    );
  }
  return <span className={cn(statusValue && `font-semibold ${statusText[statusValue]}`)}>{cell.text}</span>;
}
