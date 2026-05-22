// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useRef, useCallback, useEffect } from 'react';
import { IconGripVertical } from '@tabler/icons-react';
import type { CanvasEdge } from '../types';
import { useCanvasStore } from '../stores/canvasStore';
import { ct } from '../canvas-theme';

type EdgeLabelProps = {
  edge: CanvasEdge;
  mx: number;
  my: number;
  isSelected: boolean;
  viewportZoom: number;
};

export function EdgeLabel({ edge, mx, my, isSelected, viewportZoom }: EdgeLabelProps) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(edge.label ?? '');
  const [dragOffset, setDragOffset] = useState<{ x: number; y: number } | null>(null);
  const updateEdgeLabel = useCanvasStore((s) => s.updateEdgeLabel);
  const updateEdgeLabelOffset = useCanvasStore((s) => s.updateEdgeLabelOffset);
  const inputRef = useRef<HTMLInputElement>(null);
  const dragRef = useRef<{ startX: number; startY: number; origX: number; origY: number } | null>(null);
  // Track latest offset in a ref so onUp can read it without stale closure
  const latestOffsetRef = useRef<{ x: number; y: number } | null>(null);

  const effectiveOffset = dragOffset ?? edge.labelOffset ?? { x: 0, y: 0 };
  const lx = mx + effectiveOffset.x;
  const ly = my + effectiveOffset.y;

  const startEditing = useCallback(() => {
    setDraft(edge.label ?? '');
    setEditing(true);
    setTimeout(() => inputRef.current?.focus(), 0);
  }, [edge.label]);

  const commit = useCallback(() => {
    setEditing(false);
    updateEdgeLabel(edge.id, draft.trim() || null);
  }, [edge.id, draft, updateEdgeLabel]);

  const startDrag = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      origX: edge.labelOffset?.x ?? 0,
      origY: edge.labelOffset?.y ?? 0,
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
        updateEdgeLabelOffset(edge.id, finalOffset);
      }
      dragRef.current = null;
      latestOffsetRef.current = null;
      setDragOffset(null);
    };

    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  }, [edge.id, edge.labelOffset, viewportZoom, updateEdgeLabelOffset]);

  useEffect(() => {
    return () => { dragRef.current = null; };
  }, []);

  if (editing) {
    return (
      <foreignObject x={lx - 60} y={ly - 10} width={120} height={22} className="pointer-events-auto overflow-visible">
        <input
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === 'Enter') commit();
            if (e.key === 'Escape') { setEditing(false); setDraft(edge.label ?? ''); }
          }}
          className={`h-5 w-full rounded border border-blue-400/50 ${ct.nodeBg} px-1.5 text-center text-[10px] ${ct.text80} outline-none`}
        />
      </foreignObject>
    );
  }

  if (!edge.label && !isSelected) return null;

  return (
    <foreignObject
      x={lx - 60}
      y={ly - 10}
      width={120}
      height={22}
      className="pointer-events-auto overflow-visible"
    >
      <div className="group/label flex h-full items-center justify-center">
        <span
          className="flex size-4 shrink-0 cursor-move items-center justify-center opacity-0 transition-opacity group-hover/label:opacity-40 hover:!opacity-80"
          onMouseDown={startDrag}
        >
          <IconGripVertical className={`size-3 ${ct.text60}`} />
        </span>
        <div
          className="min-w-0 cursor-text"
          onDoubleClick={(e) => { e.stopPropagation(); startEditing(); }}
        >
          {edge.label ? (
            <span className={`select-none truncate rounded px-0.5 text-[10px] ${ct.text60} hover:${ct.text80}`}>
              {edge.label}
            </span>
          ) : (
            <span className={`select-none text-[10px] ${ct.text20} italic hover:${ct.text40}`}>
              label
            </span>
          )}
        </div>
        <span
          className="flex size-4 shrink-0 cursor-move items-center justify-center opacity-0 transition-opacity group-hover/label:opacity-40 hover:!opacity-80"
          onMouseDown={startDrag}
        >
          <IconGripVertical className={`size-3 ${ct.text60}`} />
        </span>
      </div>
    </foreignObject>
  );
}
