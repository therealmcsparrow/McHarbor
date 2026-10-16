// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// WidgetShell — one header contract, and controls that look like controls.
//
// Behaviour is unchanged; the chrome around it is not:
//
//   * The edit-mode border was 2px dashed. Dashed reads as *broken* —
//     placeholder, missing, invalid — which is exactly the wrong signal for
//     "you may move this". It is now a solid accent border plus a soft ring.
//
//   * The whole header strip — drag handle, title, size popover, remove —
//     is edit-mode chrome and only renders in edit mode. In view mode a
//     widget is its body and nothing else, so a dashboard being *read*
//     carries no controls, no titles and no borrowed vertical space.
//
//   * Size was a range slider labelled `W` and a +/− pair labelled `H`.
//     Both are kept — they are the precise and keyboard-accessible path —
//     but they move into a popover behind a labelled button instead of
//     occupying a permanent strip under every widget.
//
//   * Widget bodies each did their own header. The shell now owns the
//     title, an optional action slot and an optional last-updated stamp,
//     so two widgets side by side line up.
//
//   * The surface was `bg-card` — opaque, so it blocked the aurora and read
//     as a flat rectangle stamped onto the glass page. It is now `.mc-glass`,
//     the same material `Card` wears. Two consequences, both deliberate:
//     `.mc-glass` is unlayered and sets the `border` *shorthand*, so it beats
//     every Tailwind `border-*` utility — edit mode uses `outline-*` instead,
//     which the glass never touches. It also owns `box-shadow`, so the old
//     `shadow-sm`/`ring-2` are gone; either would replace the shadow and take
//     the inset lit edge with it.

import { type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconGripHorizontal, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { cn } from '@resources/utils/cn';
import { PERM } from '@core/auth/permissionKeys';
import { WidgetSizePopover } from './WidgetSizePopover';

type WidgetShellProps = {
  editMode: boolean;
  title: string;
  width: number;
  height: number;
  onRemove: () => void;
  onResize: (w: number, h: number) => void;
  /** Optional per-widget action, rendered in the header. */
  action?: ReactNode;
  /** Optional "updated 20s ago" stamp. Already formatted. */
  lastUpdated?: string;
  children: ReactNode;
};

export function WidgetShell({
  editMode,
  title,
  width,
  height,
  onRemove,
  onResize,
  action,
  lastUpdated,
  children,
}: WidgetShellProps) {
  const { t } = useTranslation('dashboard');

  return (
    <div
      className={cn(
        'mc-glass flex h-full flex-col overflow-hidden rounded-lg transition-[outline-color]',
        editMode && 'outline outline-2 -outline-offset-1 outline-primary/55',
      )}
    >
      {/* Header. Edit-mode only: it is the chrome for moving, resizing and
          removing a widget, and none of that applies while reading the
          dashboard. In view mode the body gets the full tile. */}
      {editMode && (
        <div className="flex shrink-0 items-center gap-2 border-b border-hairline px-3 py-1.5">
          <span
            className="widget-drag-handle -ml-1 cursor-grab text-muted-foreground/40 active:cursor-grabbing"
            aria-hidden="true"
          >
            <IconGripHorizontal className="size-4" />
          </span>

          <span className="min-w-0 flex-1 truncate text-xs font-medium text-muted-foreground">{title}</span>

          {lastUpdated && (
            <span className="shrink-0 text-[10px] text-muted-foreground/60 tabular-nums">{lastUpdated}</span>
          )}

          {action}

          <WidgetSizePopover title={title} width={width} height={height} onResize={onResize} />

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                permission={PERM.WIDGETS_MANAGE}
                type="button"
                variant="ghost"
                size="icon-sm"
                onClick={onRemove}
                aria-label={t('widget.removeWidget')}
                className="size-6 text-muted-foreground/50 hover:bg-destructive/10 hover:text-destructive"
              >
                <IconX className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('widget.removeWidget')}</TooltipContent>
          </Tooltip>
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-hidden">{children}</div>
    </div>
  );
}
