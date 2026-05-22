// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useRef, useCallback } from 'react';
import type { CanvasEdge } from '../types';
import { useCanvasStore } from '../stores/canvasStore';
import { ct } from '../canvas-theme';

/* ---------- Sniffer icon (native SVG, fixed on the curve midpoint) ---------- */

type SnifferIconProps = {
  mx: number;
  my: number;
};

export function SnifferIcon({ mx, my }: SnifferIconProps) {
  return (
    <g className="pointer-events-none">
      <circle cx={mx} cy={my} r={8} fill="rgba(127, 29, 29, 0.4)" />
      <g transform={`translate(${mx - 5}, ${my - 5})`}>
        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="#f87171" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <rect x="3" y="3" width="18" height="18" rx="2" />
          <path d="M7 3v18" />
          <path d="M17 3v18" />
          <path d="M3 7h18" />
          <path d="M3 17h18" />
        </svg>
      </g>
    </g>
  );
}

/* ---------- Sniffer name (draggable) ---------- */

type SnifferNameProps = {
  edge: CanvasEdge;
  mx: number;
  my: number;
  viewportZoom: number;
};

export function SnifferName({ edge, mx, my, viewportZoom }: SnifferNameProps) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(edge.sniffer?.name ?? '');
  const [dragOffset, setDragOffset] = useState<{ x: number; y: number } | null>(null);
  const updateEdgeSniffer = useCanvasStore((s) => s.updateEdgeSniffer);
  const updateEdgeSnifferOffset = useCanvasStore((s) => s.updateEdgeSnifferOffset);
  const dragRef = useRef<{ startX: number; startY: number; origX: number; origY: number } | null>(null);
  const latestOffsetRef = useRef<{ x: number; y: number } | null>(null);

  const effectiveOffset = dragOffset ?? edge.snifferOffset ?? { x: 0, y: 16 };
  const sx = mx + effectiveOffset.x;
  const sy = my + effectiveOffset.y;

  const commit = useCallback(() => {
    setEditing(false);
    const name = draft.trim() || 'Sniffer';
    updateEdgeSniffer(edge.id, { name });
  }, [edge.id, draft, updateEdgeSniffer]);

  const startDrag = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      origX: edge.snifferOffset?.x ?? 0,
      origY: edge.snifferOffset?.y ?? 16,
    };
    latestOffsetRef.current = null;

    const onMove = (ev: MouseEvent) => {
      if (!dragRef.current) return;
      const zoom = viewportZoom || 1;
      const dx = (ev.clientX - dragRef.current.startX) / zoom;
      const dy = (ev.clientY - dragRef.current.startY) / zoom;
      const offset = { x: dragRef.current.origX + dx, y: dragRef.current.origY + dy };
      latestOffsetRef.current = offset;
      setDragOffset(offset);
    };

    const onUp = () => {
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      const finalOffset = latestOffsetRef.current;
      if (finalOffset) {
        updateEdgeSnifferOffset(edge.id, finalOffset);
      }
      dragRef.current = null;
      latestOffsetRef.current = null;
      setDragOffset(null);
    };

    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  }, [edge.id, edge.snifferOffset, viewportZoom, updateEdgeSnifferOffset]);

  return (
    <foreignObject x={sx - 35} y={sy - 8} width={70} height={18} className="pointer-events-auto overflow-visible">
      {editing ? (
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === 'Enter') commit();
            if (e.key === 'Escape') setEditing(false);
          }}
          className={`h-4 w-full rounded border border-red-400/30 ${ct.nodeBg} px-1 text-center text-[9px] text-red-300 outline-none`}
          autoFocus
        />
      ) : (
        <div className="group/sniffer flex h-full items-center justify-center">
          <span
            className="cursor-move select-none truncate rounded bg-red-900/20 px-1.5 py-0.5 text-[9px] text-red-400"
            onMouseDown={startDrag}
            onDoubleClick={(e) => {
              e.stopPropagation();
              setDraft(edge.sniffer?.name ?? '');
              setEditing(true);
            }}
          >
            {edge.sniffer?.name ?? 'Sniffer'}
          </span>
        </div>
      )}
    </foreignObject>
  );
}
