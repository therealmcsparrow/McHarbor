// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from "react";
import {
  IconGripHorizontal,
  IconX,
  IconMinus,
  IconPlus,
} from "@tabler/icons-react";
import { Button } from "@resources/components/ui/Button";
import { cn } from "@resources/utils/cn";

type WidgetShellProps = {
  editMode: boolean;
  title: string;
  width: number;
  height: number;
  onRemove: () => void;
  onResize: (w: number, h: number) => void;
  children: ReactNode;
};

export function WidgetShell({
  editMode,
  title,
  width,
  height,
  onRemove,
  onResize,
  children,
}: WidgetShellProps) {
  return (
    <div
      className={cn(
        "flex h-full flex-col overflow-hidden",
        editMode
          ? "rounded-xl border-2 border-dashed border-primary/30 bg-card shadow-sm"
          : "rounded-lg border border-border bg-card",
      )}
    >
      {editMode && (
        <div className="widget-drag-handle flex shrink-0 items-center justify-between border-b border-border/50 px-3 py-1.5">
          <div className="flex items-center gap-2 text-muted-foreground">
            <IconGripHorizontal className="h-4 w-4 cursor-grab active:cursor-grabbing" />
            <span className="truncate text-xs font-medium text-muted-foreground/60">
              {title}
            </span>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={onRemove}
            aria-label="Remove widget"
            className="size-6 text-muted-foreground/40 hover:bg-destructive/10 hover:text-destructive"
          >
            <IconX className="h-3.5 w-3.5" />
          </Button>
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-hidden">{children}</div>

      {editMode && (
        <div className="flex shrink-0 items-center justify-between gap-4 border-t border-border/50 px-3 py-1.5">
          <div className="flex flex-1 items-center gap-2">
            <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground/30">
              W
            </span>
            <input
              type="range"
              value={width}
              min={1}
              max={12}
              step={1}
              onChange={(e) => onResize(parseInt(e.target.value, 10), height)}
              className="h-1 w-full cursor-pointer appearance-none rounded-full bg-muted-foreground/10 accent-primary [&::-moz-range-thumb]:size-3 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:bg-primary [&::-webkit-slider-thumb]:size-3 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-primary"
            />
            <span className="min-w-5 text-center text-[10px] font-semibold tabular-nums text-primary">
              {width}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <span className="mr-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground/30">
              H
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Decrease height"
              onClick={() => onResize(width, Math.max(1, height - 1))}
              className="size-5 text-muted-foreground/40 hover:bg-muted hover:text-muted-foreground/60"
            >
              <IconMinus className="h-3 w-3" />
            </Button>
            <span className="min-w-5 text-center text-[10px] font-semibold tabular-nums text-muted-foreground/50">
              {height}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Increase height"
              onClick={() => onResize(width, height + 1)}
              className="size-5 text-muted-foreground/40 hover:bg-muted hover:text-muted-foreground/60"
            >
              <IconPlus className="h-3 w-3" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
