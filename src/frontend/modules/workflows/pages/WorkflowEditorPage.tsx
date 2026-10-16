// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useCallback, useState, lazy, Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { createPortal } from 'react-dom';
import { useParams } from 'react-router';
import { IconHome } from '@tabler/icons-react';
import { Breadcrumb } from '@resources/layout/Breadcrumb';
import { Spinner } from '@resources/components/ui/Spinner';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useWorkflow, useUpdateWorkflow } from '../hooks/useWorkflows';
import { useCanvasStore } from '../stores/canvasStore';
import { useHistoryStore } from '../stores/historyStore';
import { useExecutionStore } from '../stores/executionStore';
import { WorkflowToolbar } from '../components/WorkflowToolbar';
import { SelfHostAuthorizedBadge, WorkflowSaveRefusal } from '../components/WorkflowSelfHostNotice';
import type { CanvasData } from '../types';
import { PluginMountBoundary } from '@resources/components/PluginMountBoundary';

const WorkflowCanvas = lazy(() => import('../components/WorkflowCanvas').then((m) => ({ default: m.WorkflowCanvas })));
const NodePalette = lazy(() => import('../components/NodePalette').then((m) => ({ default: m.NodePalette })));
const EditorPanel = lazy(() => import('../components/EditorPanel').then((m) => ({ default: m.EditorPanel })));

export default function WorkflowEditorPage() {
  const { t } = useTranslation('common');
  const { id } = useParams<{ id: string }>();
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

  // Activate header slot on mount, deactivate on unmount
  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  // Seeds the canvas and history zustand stores from the fetched workflow; the stores are external, and the
  // spinner gate below must wait until they hold this workflow's data.
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
      // eslint-disable-next-line react/set-state-in-effect -- readiness flag for the external store seeded just above
      setInitialized(true);
    }
  }, [workflow, initialized, initCanvas, initHistory]);

  const handleSave = useCallback(() => {
    if (!id) return;
    const data = getCanvasData();
    updateWorkflow.mutate({ id, canvasData: JSON.stringify(data) });
  }, [id, getCanvasData, updateWorkflow]);

  const handleExecute = useCallback(
    (triggerNodeId: string) => {
      if (!id) return;
      // Auto-save before executing
      const data = getCanvasData();
      updateWorkflow.mutate(
        { id, canvasData: JSON.stringify(data) },
        {
          onSuccess: () => startExecution(id, triggerNodeId),
        },
      );
    },
    [id, getCanvasData, updateWorkflow, startExecution],
  );

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
  const selectedNode = selectedNodeId ? (nodes.find((n) => n.id === selectedNodeId) ?? null) : null;
  const headerSlot = document.getElementById('header-slot');

  return (
    <div className="flex h-full flex-col">
      {/* Portal: workflow bar rendered inside the Header */}
      {headerSlot &&
        createPortal(
          <div className="flex flex-1 items-center justify-between gap-4">
            <h1 className="sr-only">{workflow.name || t('workflows.namePlaceholder')}</h1>
            <Breadcrumb
              crumbs={[
                { to: '/dashboard', label: <IconHome className="size-3" /> },
                { label: t('workflows.title'), to: '/workflows' },
                { label: workflow.name || t('workflows.namePlaceholder') },
              ]}
              className="min-w-0 flex-1"
            />

            {/* Right: toolbar */}
            <div className="flex shrink-0 items-center gap-2">
              <SelfHostAuthorizedBadge authorized={workflow.selfHostAuthorized} />
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
          headerSlot,
        )}

      <WorkflowSaveRefusal error={updateWorkflow.error} />

      {/* Main layout */}
      <div className="flex flex-1 overflow-hidden">
        {/* Left: palette */}
        <PluginMountBoundary>
          <Suspense fallback={<div className="w-72 border-r border-border bg-card/50" />}>
            <NodePalette />
          </Suspense>
        </PluginMountBoundary>

        {/* Center: canvas */}
        <div className="relative flex-1">
          <PluginMountBoundary>
            <Suspense
              fallback={
                <div className="flex h-full items-center justify-center">
                  <Spinner size="lg" />
                </div>
              }
            >
              <WorkflowCanvas onExecute={handleExecute} />
            </Suspense>
          </PluginMountBoundary>
        </div>

        {/* Right: editor panel (always visible) */}
        {selectedNode || debugMessages.length > 0 || errors.length > 0 ? (
          <PluginMountBoundary>
            <Suspense fallback={<div className="border-l border-border bg-card/50" />}>
              <EditorPanel
                selectedNode={selectedNode}
                debugMessages={debugMessages}
                errors={errors}
                onClearDebug={clearDebug}
                onClearErrors={clearErrors}
              />
            </Suspense>
          </PluginMountBoundary>
        ) : (
          <div className="border-l border-border bg-card/30" />
        )}
      </div>
    </div>
  );
}
