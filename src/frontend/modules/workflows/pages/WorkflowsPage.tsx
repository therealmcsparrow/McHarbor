// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import {
  IconPlus,
  IconTrash,
  IconEdit,
  IconPencil,
  IconGitMerge,
  IconPower,
  IconHistory,
  IconDownload,
  IconBolt,
} from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { PageBody } from '@resources/layout/PageBody';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { timeAgo } from '@resources/utils/format';
import {
  useWorkflows,
  useUpdateWorkflow,
  useCreateWorkflow,
  useDeleteWorkflow,
  useExportWorkflow,
  useImportWorkflow,
} from '../hooks/useWorkflows';
import { CreateWorkflowDialog } from '../components/CreateWorkflowDialog';
import { EditWorkflowDetailsDialog } from '../components/EditWorkflowDetailsDialog';
import { WorkflowImportButton } from '../components/WorkflowImportButton';
import { ct } from '../canvas-theme';
import { PERM } from '@core/auth/permissionKeys';
import type { Workflow } from '../types';

const STATUS_VARIANTS: Record<string, 'success' | 'secondary' | 'warning' | 'destructive'> = {
  active: 'success',
  draft: 'secondary',
  archived: 'warning',
};

export default function WorkflowsPage() {
  const { t } = useTranslation('common');
  const navigate = useNavigate();
  const { data: workflows = [], isLoading } = useWorkflows();
  const createWorkflow = useCreateWorkflow();
  const updateWorkflow = useUpdateWorkflow();
  const deleteWorkflow = useDeleteWorkflow();
  const exportWorkflow = useExportWorkflow();
  const importWorkflow = useImportWorkflow();

  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const [editingWorkflow, setEditingWorkflow] = useState<Workflow | null>(null);

  if (isLoading) {
    return (
      <div className="flex h-full min-h-0 flex-col gap-6">
        <PageHeader title={t('workflows.title')} description={t('workflows.descriptionShort')} headingLeaf />
        <div className="flex h-32 items-center justify-center">
          <Spinner />
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-6">
      <PageHeader
        headingLeaf
        title={t('workflows.title')}
        description={t('workflows.description')}
        actions={
          <>
            <WorkflowImportButton importWorkflow={importWorkflow} />
            {/* Custom nodes are authored per install, not per workflow, so the
                entry point sits beside the workflow list rather than inside
                one editor. */}
            <Button
              permission={PERM.CUSTOM_NODES_VIEW}
              variant="outline"
              onClick={() => navigate('/workflows/custom-nodes')}
            >
              <IconBolt className="size-4" /> {t('customNodes.title')}
            </Button>
            <Button permission={PERM.WORKFLOWS_MANAGE} onClick={() => setCreateOpen(true)}>
              <IconPlus className="size-4" /> {t('workflows.newWorkflow')}
            </Button>
          </>
        }
      />

      <PageBody className="gap-6">
        {workflows.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-16">
            <IconGitMerge className="size-12 text-muted-foreground/30" />
            <p className="mt-4 text-sm text-muted-foreground">{t('workflows.noWorkflows')}</p>
            <Button className="mt-4" onClick={() => setCreateOpen(true)}>
              <IconPlus className="size-4" /> {t('workflows.createFirst')}
            </Button>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {workflows.map((wf) => (
              <div
                key={wf.id}
                className="group mc-glass relative flex flex-col rounded-xl p-4 transition-colors hover:border-primary/30"
              >
                <div className="flex items-start justify-between">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5">
                      <h3 className="truncate font-medium text-foreground">{wf.name}</h3>
                      <Button
                        permission={PERM.WORKFLOWS_MANAGE}
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t('workflows.editDetailsAria')}
                        onClick={(e) => {
                          e.stopPropagation();
                          setEditingWorkflow(wf);
                        }}
                      >
                        <IconPencil className="size-3.5" />
                      </Button>
                    </div>
                    <p className="mt-1 truncate text-sm text-muted-foreground">
                      {wf.description || t('workflows.noDescription')}
                    </p>
                  </div>
                  <div className="ml-3 flex shrink-0 items-center gap-1.5">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={(e) => {
                        e.stopPropagation();
                        exportWorkflow.mutate({ id: wf.id, name: wf.name });
                      }}
                      disabled={exportWorkflow.isPending}
                    >
                      <IconDownload className="size-3.5" /> {t('workflows.export')}
                    </Button>
                    <Badge variant={STATUS_VARIANTS[wf.status] ?? 'secondary'}>{wf.status}</Badge>
                  </div>
                </div>

                <div className="mt-4 flex items-center gap-2 text-xs text-muted-foreground">
                  {wf.lastRunAt && <span>{t('workflows.lastRun', { time: timeAgo(wf.lastRunAt) })}</span>}
                  {!wf.lastRunAt && <span>{t('workflows.neverRun')}</span>}
                  <span>&middot;</span>
                  <span>{t('workflows.updated', { time: timeAgo(wf.updatedAt) })}</span>
                </div>

                <div className="mt-3 flex flex-wrap items-center gap-1.5">
                  <Button size="sm" onClick={() => navigate(`/workflows/${wf.id}`)}>
                    <IconEdit className="size-3.5" /> {t('actions.edit')}
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => navigate(`/workflows/${wf.id}/runs`)}>
                    <IconHistory className="size-3.5" /> {t('workflows.runs')}
                  </Button>
                  <Button
                    size="sm"
                    variant={wf.status === 'active' ? 'default' : 'outline'}
                    className={wf.status === 'active' ? `bg-success hover:bg-success ${ct.activeBtnText}` : ''}
                    onClick={(e) => {
                      e.stopPropagation();
                      const newStatus = wf.status === 'active' ? 'draft' : 'active';
                      updateWorkflow.mutate({ id: wf.id, status: newStatus });
                    }}
                    disabled={updateWorkflow.isPending}
                  >
                    <IconPower className="size-3.5" />{' '}
                    {wf.status === 'active' ? t('workflows.active') : t('workflows.inactive')}
                  </Button>
                  <div className="flex-1" />
                  <Button
                    permission={PERM.WORKFLOWS_DELETE}
                    size="sm"
                    variant="ghost"
                    onClick={(e) => {
                      e.stopPropagation();
                      setConfirmTarget(wf.id);
                    }}
                  >
                    <IconTrash className="size-3.5 text-destructive" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </PageBody>

      <CreateWorkflowDialog open={createOpen} onOpenChange={setCreateOpen} createWorkflow={createWorkflow} />

      <EditWorkflowDetailsDialog
        workflow={editingWorkflow}
        onOpenChange={(nextOpen) => !nextOpen && setEditingWorkflow(null)}
      />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(nextOpen) => !nextOpen && setConfirmTarget(null)}
        title={t('workflows.deleteTitle')}
        description={t('workflows.deleteDescription')}
        confirmLabel={t('actions.delete')}
        onConfirm={() => {
          if (confirmTarget) deleteWorkflow.mutate(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={deleteWorkflow.isPending}
      />
    </div>
  );
}
