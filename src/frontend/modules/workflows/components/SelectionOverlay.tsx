// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CanvasViewport } from '../types';

interface SelectionOverlayProps {
  rect: { startX: number; startY: number; endX: number; endY: number };
  viewport: CanvasViewport;
}

export function SelectionOverlay({ rect, viewport }: SelectionOverlayProps) {
  return (
    <div
      style={{
        transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.zoom})`,
        transformOrigin: '0 0',
      }}
    >
      <div
        className="pointer-events-none absolute rounded-sm border border-blue-400/50 bg-blue-400/10"
        style={{
          left: Math.min(rect.startX, rect.endX),
          top: Math.min(rect.startY, rect.endY),
          width: Math.abs(rect.endX - rect.startX),
          height: Math.abs(rect.endY - rect.startY),
        }}
      />
    </div>
  );
}
