// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import type { CanvasViewport } from '../types';
import { ct } from '../canvas-theme';

interface SelectionOverlayProps {
  rect: { startX: number; startY: number; endX: number; endY: number };
  viewport: CanvasViewport;
}

export function SelectionOverlay({ rect, viewport: canvasViewport }: SelectionOverlayProps) {
  return (
    <div
      className="[transform-origin:0_0] [transform:var(--vt)]"
      style={cssVars({ vt: `translate(${canvasViewport.x}px, ${canvasViewport.y}px) scale(${canvasViewport.zoom})` })}
    >
      <div
        className={`pointer-events-none absolute top-(--y) left-(--x) h-(--h) w-(--w) rounded-sm ${ct.selection}`}
        style={cssVars({
          x: `${Math.min(rect.startX, rect.endX)}px`,
          y: `${Math.min(rect.startY, rect.endY)}px`,
          w: `${Math.abs(rect.endX - rect.startX)}px`,
          h: `${Math.abs(rect.endY - rect.startY)}px`,
        })}
      />
    </div>
  );
}
