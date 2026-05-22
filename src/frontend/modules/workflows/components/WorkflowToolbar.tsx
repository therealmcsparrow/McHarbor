// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconArrowBackUp,
  IconArrowForwardUp,
  IconZoomIn,
  IconZoomOut,
  IconDeviceFloppy,
  IconPlayerStop,
  IconPower,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { useCanvasStore } from '../stores/canvasStore';
import { useHistoryStore } from '../stores/historyStore';
import { ct } from '../canvas-theme';

type WorkflowToolbarProps = {
  onSave: () => void;
  isSaving: boolean;
  isExecuting?: boolean;
  onStop?: () => void;
  status?: string;
  onToggleActive?: () => void;
  isTogglingActive?: boolean;
};

export function WorkflowToolbar({ onSave, isSaving, isExecuting, onStop, status, onToggleActive, isTogglingActive }: WorkflowToolbarProps) {
  const { t } = useTranslation('common');
  const viewport = useCanvasStore((s) => s.viewport);
  const setViewport = useCanvasStore((s) => s.setViewport);
  const undo = useHistoryStore((s) => s.undo);
  const redo = useHistoryStore((s) => s.redo);
  const canUndo = useHistoryStore((s) => s.undoStack.length > 0);
  const canRedo = useHistoryStore((s) => s.redoStack.length > 0);

  const zoomIn = () => {
    const z = Math.min(2, viewport.zoom + 0.1);
    setViewport({ ...viewport, zoom: Math.round(z * 100) / 100 });
  };

  const zoomOut = () => {
    const z = Math.max(0.25, viewport.zoom - 0.1);
    setViewport({ ...viewport, zoom: Math.round(z * 100) / 100 });
  };

  const zoomReset = () => {
    setViewport({ ...viewport, zoom: 1 });
  };

  return (
    <div className="flex items-center gap-1">
      {/* Zoom controls */}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button size="sm" variant="ghost" onClick={zoomOut} className="size-7 p-0">
            <IconZoomOut className="size-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('workflows.zoomOut')}</TooltipContent>
      </Tooltip>

      <Button
        variant="ghost"
        size="sm"
        onClick={zoomReset}
        aria-label={t('workflows.resetZoom')}
        className="min-w-[3rem] px-1.5"
      >
        {Math.round(viewport.zoom * 100)}%
      </Button>

      <Tooltip>
        <TooltipTrigger asChild>
          <Button size="sm" variant="ghost" onClick={zoomIn} className="size-7 p-0">
            <IconZoomIn className="size-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('workflows.zoomIn')}</TooltipContent>
      </Tooltip>

      <div className="mx-1.5 h-4 w-px bg-border" />

      {/* Undo / Redo */}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button size="sm" variant="ghost" onClick={undo} disabled={!canUndo} className="size-7 p-0">
            <IconArrowBackUp className="size-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('workflows.undo')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger asChild>
          <Button size="sm" variant="ghost" onClick={redo} disabled={!canRedo} className="size-7 p-0">
            <IconArrowForwardUp className="size-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('workflows.redo')}</TooltipContent>
      </Tooltip>

      <div className="mx-1.5 h-4 w-px bg-border" />

      {/* Activate / Deactivate */}
      {onToggleActive && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant={status === 'active' ? 'default' : 'outline'}
              onClick={onToggleActive}
              disabled={isTogglingActive}
              className={status === 'active'
                ? `bg-emerald-600 hover:bg-emerald-700 ${ct.activeBtnText}`
                : ''}
            >
              <IconPower className="size-3.5" />
              {status === 'active' ? t('workflows.active') : t('workflows.inactive')}
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {status === 'active'
              ? t('workflows.activeTooltip')
              : t('workflows.inactiveTooltip')}
          </TooltipContent>
        </Tooltip>
      )}

      {/* Save */}
      <Button size="sm" onClick={onSave} disabled={isSaving}>
        <IconDeviceFloppy className="size-3.5" />
        {isSaving ? t('workflows.saving') : t('actions.save')}
      </Button>

      {/* Stop execution */}
      {isExecuting && onStop && (
        <>
          <div className="mx-1.5 h-4 w-px bg-border" />
          <Button size="sm" variant="destructive" onClick={onStop}>
            <IconPlayerStop className="size-3.5" />
            {t('workflows.stopExecution')}
          </Button>
        </>
      )}
    </div>
  );
}

