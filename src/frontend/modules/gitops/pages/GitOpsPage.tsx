// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { PageHeader } from '@resources/layout/PageHeader';
import { PageBody } from '@resources/layout/PageBody';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { useGitRepos } from '@resources/hooks/useGitRepos';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import {
  useDeletePipeline,
  useGitOpsApprovals,
  useGitOpsPipelines,
  useGitOpsPRPreviews,
  useGitOpsPromotions,
  useResolveApproval,
} from '../hooks/useGitOps';
import { PipelineFormDialog } from '../components/PipelineFormDialog';
import { PreviewsPanel } from '../components/PreviewsPanel';
import { WebhookSecretPanel } from '../components/WebhookSecretPanel';
import { PromoteDialog } from '../components/PromoteDialog';
import { ApprovalsBanner } from '../components/ApprovalsBanner';
import { PipelineCard } from '../components/PipelineCard';
import { PipelineStages } from '../components/PipelineStages';
import { DeploymentHistory } from '../components/DeploymentHistory';
import type { GitOpsPipeline } from '../types';
import { PERM } from '@core/auth/permissionKeys';

export function GitOpsPage() {
  const { t } = useTranslation('gitops');
  const pipelines = useGitOpsPipelines();
  const approvals = useGitOpsApprovals();
  const previews = useGitOpsPRPreviews();
  const repos = useGitRepos();
  const envs = useEnvironmentList();
  const remove = useDeletePipeline(t);
  const resolve = useResolveApproval(t);

  const [formOpen, setFormOpen] = useState(false);
  const [editingPipeline, setEditingPipeline] = useState<GitOpsPipeline | null>(null);
  const [promoteOpen, setPromoteOpen] = useState(false);
  const [promotePipeline, setPromotePipeline] = useState<GitOpsPipeline | null>(null);
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);
  const [selectedPipelineId, setSelectedPipelineId] = useState<string | null>(null);

  const repoOptions = useMemo(() => (repos.data ?? []).map((r) => ({ id: r.id, name: r.name })), [repos.data]);
  const envOptions = useMemo(
    () =>
      (envs.data ?? []).map((e) => ({
        id: e.id,
        name: (e as { name?: string }).name ?? e.id,
      })),
    [envs.data],
  );

  const promotions = useGitOpsPromotions(selectedPipelineId ?? undefined);

  const selectedPipeline = useMemo(
    () => (pipelines.data ?? []).find((p) => p.id === selectedPipelineId) ?? null,
    [pipelines.data, selectedPipelineId],
  );

  if (pipelines.isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  const list = pipelines.data ?? [];

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <PageHeader
        headingLeaf
        title={t('title')}
        description={t('description')}
        actions={
          <Button
            permission={PERM.GITOPS_MANAGE}
            onClick={() => {
              setEditingPipeline(null);
              setFormOpen(true);
            }}
          >
            <IconPlus className="mr-2 size-4" />
            {t('addPipeline')}
          </Button>
        }
      />

      <PageBody className="gap-4">
        {/* Approvals banner */}
        {approvals.data && approvals.data.length > 0 && (
          <ApprovalsBanner approvals={approvals.data} resolve={resolve} />
        )}

        <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
          {/* Pipeline list */}
          <div className="lg:col-span-1">
            <h3 className="mb-2 text-sm font-semibold text-foreground">{t('title')}</h3>
            {list.length === 0 ? (
              <div className="rounded-lg border border-dashed border-border bg-muted/30 p-6 text-center">
                <p className="text-sm text-muted-foreground">{t('noPipelines')}</p>
                <Button
                  permission={PERM.GITOPS_MANAGE}
                  className="mt-3"
                  onClick={() => {
                    setEditingPipeline(null);
                    setFormOpen(true);
                  }}
                >
                  {t('emptyStateAction')}
                </Button>
              </div>
            ) : (
              <div className="space-y-2">
                {list.map((p) => (
                  <PipelineCard
                    key={p.id}
                    pipeline={p}
                    selected={p.id === selectedPipelineId}
                    onSelect={() => setSelectedPipelineId(p.id)}
                    onEdit={() => {
                      setEditingPipeline(p);
                      setFormOpen(true);
                    }}
                    onPromote={() => {
                      setPromotePipeline(p);
                      setPromoteOpen(true);
                    }}
                    onDelete={() => setDeleteConfirmId(p.id)}
                  />
                ))}
              </div>
            )}
          </div>

          {/* Pipeline detail */}
          <div className="space-y-4 lg:col-span-2">
            {selectedPipeline ? (
              <>
                <PipelineStages pipeline={selectedPipeline} t={t} />
                <DeploymentHistory promotions={promotions.data ?? []} isLoading={promotions.isLoading} t={t} />
              </>
            ) : (
              <div className="rounded-lg border border-dashed border-border bg-muted/30 p-6 text-center text-sm text-muted-foreground">
                {list.length > 0 ? t('selectPipeline') : t('noPipelinesDescription')}
              </div>
            )}

            <PreviewsPanel previews={previews.data ?? []} />
            <WebhookSecretPanel repos={repoOptions} />
          </div>
        </div>
      </PageBody>

      <PipelineFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        pipeline={editingPipeline}
        repos={repoOptions}
        environments={envOptions}
      />

      {promotePipeline && <PromoteDialog open={promoteOpen} onOpenChange={setPromoteOpen} pipeline={promotePipeline} />}

      <ConfirmDialog
        open={deleteConfirmId !== null}
        onOpenChange={(isOpen) => !isOpen && setDeleteConfirmId(null)}
        title={t('formActions.delete')}
        description={t('formActions.delete')}
        onConfirm={() => {
          if (deleteConfirmId) {
            remove.mutate(deleteConfirmId, {
              onSuccess: () => setDeleteConfirmId(null),
            });
          }
        }}
        confirmLabel={t('formActions.delete')}
        variant="destructive"
      />
    </div>
  );
}
