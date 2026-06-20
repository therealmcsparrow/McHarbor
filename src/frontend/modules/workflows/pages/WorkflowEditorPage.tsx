// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useCallback, useState, lazy, Suspense, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { createPortal } from 'react-dom';
import { useParams, useNavigate } from 'react-router';
import { IconArrowLeft, IconCircleCheck, IconExclamationCircle, IconLoader2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useWorkflow, useUpdateWorkflow } from '../hooks/useWorkflows';
import { useCanvasStore } from '../stores/canvasStore';
import { useHistoryStore } from '../stores/historyStore';
import { useExecutionStore } from '../stores/executionStore';
import { WorkflowToolbar } from '../components/WorkflowToolbar';
import type { CanvasData } from '../types';

const WorkflowCanvas = lazy(() => import('../components/WorkflowCanvas').then((m) => ({ default: m.WorkflowCanvas })));
const NodePalette = lazy(() => import('../components/NodePalette').then((m) => ({ default: m.NodePalette })));
const EditorPanel = lazy(() => import('../components/EditorPanel').then((m) => ({ default: m.EditorPanel })));

type MetadataStatus = 'idle' | 'saving' | 'saved' | 'error';

export default function WorkflowEditorPage() {
  const { t } = useTranslation('common');
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: workflow, isLoading } = useWorkflow(id ?? '');
  const updateWorkflow = useUpdateWorkflow();
  const initCanvas = useCanvasStore((s) => s.initCanvas);
  const getCanvasData = useCanvasStore((s) => s.getCanvasData);
  const selectedNodeIds = useCanvasStore((s) => s.selectedNodeIds);
  const nodes = useCanvasStore((s) => s.nodes);
  const initHistory = useHistoryStore((s) => s.init);
  const isExecuting = useExecutionStore((s) => s.isExecuting);
  const startExecution = useExecutionStore((s) => s.startExecution);
  const stopExecution = useExecutionStore((s) => s.stopExecution);
  const resetExecution = useExecutionStore((s) => s.resetExecution);
  const debugMessages = useExecutionStore((s) => s.debugMessages);
  const errors = useExecutionStore((s) => s.errors);
  const clearDebug = useExecutionStore((s) => s.clearDebug);
  const clearErrors = useExecutionStore((s) => s.clearErrors);
  const subscribeLive = useExecutionStore((s) => s.subscribeLive);
  const unsubscribeLive = useExecutionStore((s) => s.unsubscribeLive);
  const setHeaderActive = useHeaderSlot((s) => s.setActive);
  const [initialized, setInitialized] = useState(false);

  // Editable metadata state — kept in sync with the loaded workflow.
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [metadataStatus, setMetadataStatus] = useState<MetadataStatus>('idle');
  const [metadataError, setMetadataError] = useState<string | null>(null);
  const lastSavedNameRef = useRef('');
  const lastSavedDescriptionRef = useRef('');
  const metadataDirtyRef = useRef(false);

  // Activate header slot on mount, deactivate on unmount
  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  useEffect(() => {
    if (workflow && !initialized) {
      let canvasData: CanvasData | null = null;
      try {
        canvasData = JSON.parse(workflow.canvasData) as CanvasData;
      } catch {
        // empty canvas
      }
      initCanvas(canvasData);
      initHistory();
      setInitialized(true);
    }
  }, [workflow, initialized, initCanvas, initHistory]);

  // Hydrate the editable fields whenever the workflow record loads or changes.
  useEffect(() => {
    if (!workflow) return;
    const serverName = workflow.name ?? '';
    const serverDescription = workflow.description ?? '';
    // Only sync from server when the user is not currently editing
    // (avoid clobbering in-flight keystrokes on a background refresh).
    if (!metadataDirtyRef.current) {
      setName(serverName);
      setDescription(serverDescription);
      lastSavedNameRef.current = serverName;
      lastSavedDescriptionRef.current = serverDescription;
      setMetadataStatus('idle');
      setMetadataError(null);
    }
  }, [workflow]);

  // Persist workflow metadata (title + description) when dirty. Called by
  // blur handlers, Enter/Escape on the name field, and the toolbar Save
  // button so dirty fields never get silently dropped.
  const persistMetadata = useCallback(
    (overrides?: { name?: string; description?: string }) => {
      if (!id) return;
      const nextName = (overrides?.name ?? name).trim();
      const nextDescription = (overrides?.description ?? description).trim();
      const nameChanged = nextName !== lastSavedNameRef.current;
      const descriptionChanged = nextDescription !== lastSavedDescriptionRef.current;
      if (!nameChanged && !descriptionChanged) {
        metadataDirtyRef.current = false;
        setMetadataStatus('idle');
        setMetadataError(null);
        return;
      }
      if (nextName === '') {
        setMetadataStatus('error');
        setMetadataError(t('workflows.nameRequired'));
        return;
      }
      setMetadataStatus('saving');
      setMetadataError(null);
      updateWorkflow.mutate(
        {
          id,
          name: nameChanged ? nextName : undefined,
          description: descriptionChanged ? nextDescription : undefined,
        },
        {
          onSuccess: () => {
            lastSavedNameRef.current = nextName;
            lastSavedDescriptionRef.current = nextDescription;
            metadataDirtyRef.current = false;
            setMetadataStatus('saved');
          },
          onError: (err) => {
            setMetadataStatus('error');
            setMetadataError((err as Error).message || t('workflows.metadataSaveFailed'));
          },
        },
      );
    },
    [id, name, description, updateWorkflow, t],
  );

  const handleNameChange = useCallback((value: string) => {
    setName(value);
    metadataDirtyRef.current = true;
    setMetadataStatus((current) => (current === 'error' ? 'idle' : current));
    setMetadataError(null);
  }, []);

  const handleNameBlur = useCallback(() => {
    persistMetadata();
  }, [persistMetadata]);

  const handleNameKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        (e.currentTarget as HTMLInputElement).blur();
      } else if (e.key === 'Escape') {
        e.preventDefault();
        setName(lastSavedNameRef.current);
        metadataDirtyRef.current = false;
        setMetadataStatus('idle');
        setMetadataError(null);
        (e.currentTarget as HTMLInputElement).blur();
      }
    },
    [],
  );

  const handleDescriptionChange = useCallback((value: string) => {
    setDescription(value);
    metadataDirtyRef.current = true;
    setMetadataStatus((current) => (current === 'error' ? 'idle' : current));
    setMetadataError(null);
  }, []);

  const handleDescriptionBlur = useCallback(() => {
    persistMetadata();
  }, [persistMetadata]);

  const handleDescriptionKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        setDescription(lastSavedDescriptionRef.current);
        metadataDirtyRef.current = false;
        setMetadataStatus('idle');
        setMetadataError(null);
        (e.currentTarget as HTMLTextAreaElement).blur();
      }
      // Enter intentionally inserts a newline inside the textarea.
    },
    [],
  );

  const handleSave = useCallback(() => {
    if (!id) return;
    const data = getCanvasData();
    // Flush any dirty metadata fields alongside the canvas save so users
    // can't lose title/description edits by clicking the main Save button.
    persistMetadata();
    updateWorkflow.mutate({ id, canvasData: JSON.stringify(data) });
  }, [id, getCanvasData, updateWorkflow, persistMetadata]);

  const handleExecute = useCallback((triggerNodeId: string) => {
    if (!id) return;
    // Auto-save before executing
    const data = getCanvasData();
    persistMetadata();
    updateWorkflow.mutate({ id, canvasData: JSON.stringify(data) }, {
      onSuccess: () => startExecution(id, triggerNodeId),
    });
  }, [id, getCanvasData, updateWorkflow, startExecution, persistMetadata]);

  const handleStop = useCallback(() => {
    stopExecution();
  }, [stopExecution]);

  const handleToggleActive = useCallback(() => {
    if (!id || !workflow) return;
    const newStatus = workflow.status === 'active' ? 'draft' : 'active';
    updateWorkflow.mutate({ id, status: newStatus });
  }, [id, workflow, updateWorkflow]);

  // Subscribe to live background execution events
  useEffect(() => {
    if (id && initialized) {
      subscribeLive(id);
    }
    return () => unsubscribeLive();
  }, [id, initialized, subscribeLive, unsubscribeLive]);

  // Cleanup on unmount
  useEffect(() => {
    return () => resetExecution();
  }, [resetExecution]);

  // Persist any dirty metadata when the editor unmounts so navigation
  // away doesn't silently drop unsaved name/description edits.
  useEffect(() => {
    return () => {
      if (metadataDirtyRef.current) {
        metadataDirtyRef.current = false;
      }
    };
  }, []);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        handleSave();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [handleSave]);

  if (isLoading || !initialized) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!workflow) {
    return <div className="py-12 text-center text-muted-foreground">{t('workflows.workflowNotFound')}</div>;
  }

  const selectedNodeId = selectedNodeIds.size === 1 ? [...selectedNodeIds][0] : null;
  const selectedNode = selectedNodeId ? nodes.find((n) => n.id === selectedNodeId) ?? null : null;
  const headerSlot = document.getElementById('header-slot');

  const metadataStatusLabel =
    metadataStatus === 'saving'
      ? t('workflows.saving')
      : metadataStatus === 'saved'
        ? t('workflows.metadataSaved')
        : metadataStatus === 'error'
          ? (metadataError ?? t('workflows.metadataSaveFailed'))
          : null;

  return (
    <div className="flex h-full flex-col">
      {/* Portal: workflow bar rendered inside the Header */}
      {headerSlot && createPortal(
        <div className="flex flex-1 items-center justify-between gap-4">
          {/* Left: back + editable title + description */}
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={() => navigate('/workflows')}
                  aria-label={t('workflows.backToWorkflows')}
                  className="size-8"
                >
                  <IconArrowLeft className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('workflows.backToWorkflows')}</TooltipContent>
            </Tooltip>
            <div className="h-8 w-px shrink-0 bg-border" />
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  value={name}
                  onChange={(e) => handleNameChange(e.target.value)}
                  onBlur={handleNameBlur}
                  onKeyDown={handleNameKeyDown}
                  placeholder={t('workflows.namePlaceholder')}
                  aria-label={t('workflows.nameAria')}
                  maxLength={120}
                  className="h-7 min-w-0 max-w-[28rem] flex-1 truncate rounded-md border border-transparent bg-transparent px-2 text-sm font-semibold text-foreground outline-none transition-colors hover:border-border focus:border-primary focus:bg-card"
                />
                <MetadataStatusBadge status={metadataStatus} label={metadataStatusLabel} />
              </div>
              <textarea
                value={description}
                onChange={(e) => handleDescriptionChange(e.target.value)}
                onBlur={handleDescriptionBlur}
                onKeyDown={handleDescriptionKeyDown}
                placeholder={t('workflows.descriptionPlaceholder')}
                aria-label={t('workflows.descriptionAria')}
                rows={1}
                className="block w-full resize-none rounded-md border border-transparent bg-transparent px-2 py-1 text-xs text-muted-foreground outline-none transition-colors hover:border-border focus:border-primary focus:bg-card"
              />
            </div>
          </div>

          {/* Right: toolbar */}
          <div className="shrink-0">
            <WorkflowToolbar
              onSave={handleSave}
              isSaving={updateWorkflow.isPending}
              isExecuting={isExecuting}
              onStop={handleStop}
              status={workflow.status}
              onToggleActive={handleToggleActive}
              isTogglingActive={updateWorkflow.isPending}
            />
          </div>
        </div>,
        headerSlot
      )}

      {/* Main layout */}
      <div className="flex flex-1 overflow-hidden">
        {/* Left: palette */}
        <Suspense fallback={<div className="w-72 border-r border-border bg-card/50" />}>
          <NodePalette />
        </Suspense>

        {/* Center: canvas */}
        <div className="flex-1 relative">
          <Suspense
            fallback={
              <div className="flex h-full items-center justify-center">
                <Spinner size="lg" />
              </div>
            }
          >
            <WorkflowCanvas onExecute={handleExecute} />
          </Suspense>
        </div>

        {/* Right: editor panel (always visible) */}
        {selectedNode || debugMessages.length > 0 || errors.length > 0 ? (
          <Suspense fallback={<div className="w-[420px] border-l border-border bg-card/50" />}>
            <EditorPanel
              selectedNode={selectedNode}
              debugMessages={debugMessages}
              errors={errors}
              onClearDebug={clearDebug}
              onClearErrors={clearErrors}
            />
          </Suspense>
        ) : (
          <div className="w-[420px] border-l border-border bg-card/30" />
        )}
      </div>
    </div>
  );
}

function MetadataStatusBadge({ status, label }: { status: MetadataStatus; label: string | null }) {
  if (status === 'idle' || !label) return null;
  const tone =
    status === 'saving'
      ? 'text-muted-foreground'
      : status === 'saved'
        ? 'text-emerald-600 dark:text-emerald-400'
        : 'text-destructive';
  const icon =
    status === 'saving' ? (
      <IconLoader2 className="size-3.5 animate-spin" />
    ) : status === 'saved' ? (
      <IconCircleCheck className="size-3.5" />
    ) : (
      <IconExclamationCircle className="size-3.5" />
    );
  return (
    <span
      className={`flex shrink-0 items-center gap-1 text-xs ${tone}`}
      role={status === 'error' ? 'alert' : 'status'}
    >
      {icon}
      <span>{label}</span>
    </span>
  );
}

