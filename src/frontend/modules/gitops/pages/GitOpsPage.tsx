// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconRocket, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { PageHeader } from '@resources/layout/PageHeader';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { useGitRepos } from '@modules/git/hooks/useGitRepos';
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
import { PromoteDialog } from '../components/PromoteDialog';
import type { GitOpsPipeline, GitOpsStage } from '../types';

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

  const repoOptions = useMemo(
    () =>
      (repos.data ?? []).map((r) => ({ id: r.id, name: r.name })),
    [repos.data],
  );
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
    () =>
      (pipelines.data ?? []).find((p) => p.id === selectedPipelineId) ?? null,
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
    <div className="space-y-4">
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          <Button
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

      {/* Approvals banner */}
      {approvals.data && approvals.data.length > 0 && (
        <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-4">
          <h3 className="mb-2 text-sm font-semibold text-foreground">
            {t('approvals')}
          </h3>
          <div className="space-y-2">
            {approvals.data.map((a) => (
              <div
                key={a.id}
                className="flex items-center justify-between rounded-md border border-border bg-card px-3 py-2 text-sm"
              >
                <div className="flex-1 min-w-0">
                  <div className="truncate font-medium">
                    {a.pipelineId} → {a.stageId}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {a.commitSha} · {a.requestedBy} · {a.requestedAt}
                  </div>
                </div>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    onClick={() =>
                      resolve.mutate({
                        id: a.id,
                        action: 'approve',
                        input: { note: '', resolvedBy: '' },
                      })
                    }
                    disabled={resolve.isPending}
                  >
                    {t('approve')}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      resolve.mutate({
                        id: a.id,
                        action: 'reject',
                        input: { note: '', resolvedBy: '' },
                      })
                    }
                    disabled={resolve.isPending}
                  >
                    {t('reject')}
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        {/* Pipeline list */}
        <div className="lg:col-span-1">
          <h3 className="mb-2 text-sm font-semibold text-foreground">
            {t('title')}
          </h3>
          {list.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border bg-muted/30 p-6 text-center">
              <p className="text-sm text-muted-foreground">
                {t('noPipelines')}
              </p>
              <Button
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
        <div className="lg:col-span-2 space-y-4">
          {selectedPipeline ? (
            <>
              <PipelineStages pipeline={selectedPipeline} t={t} />
              <DeploymentHistory
                promotions={promotions.data ?? []}
                isLoading={promotions.isLoading}
                t={t}
              />
            </>
          ) : (
            <div className="rounded-lg border border-dashed border-border bg-muted/30 p-6 text-center text-sm text-muted-foreground">
              {list.length > 0 ? 'Select a pipeline' : t('noPipelinesDescription')}
            </div>
          )}

          {/* PR Previews */}
          {previews.data && previews.data.length > 0 && (
            <div>
              <h3 className="mb-2 text-sm font-semibold text-foreground">
                {t('previews')}
              </h3>
              <div className="space-y-2">
                {previews.data.map((p) => (
                  <div
                    key={p.id}
                    className="rounded-md border border-border bg-card px-3 py-2 text-sm"
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-medium">
                        PR #{p.prNumber} — {p.prTitle}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {p.status}
                      </span>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {p.sourceBranch} → {p.targetBranch} · {p.commitSha}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      <PipelineFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        pipeline={editingPipeline}
        repos={repoOptions}
        environments={envOptions}
      />

      {promotePipeline && (
        <PromoteDialog
          open={promoteOpen}
          onOpenChange={setPromoteOpen}
          pipeline={promotePipeline}
        />
      )}

      <ConfirmDialog
        open={deleteConfirmId !== null}
        onOpenChange={(open) => !open && setDeleteConfirmId(null)}
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

function PipelineCard({
  pipeline,
  selected,
  onSelect,
  onEdit,
  onPromote,
  onDelete,
}: {
  pipeline: GitOpsPipeline;
  selected: boolean;
  onSelect: () => void;
  onEdit: () => void;
  onPromote: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation('gitops');
  const triggerLabel =
    pipeline.triggerType === 'auto'
      ? t('triggerAuto')
      : pipeline.triggerType === 'manual'
        ? t('triggerManual')
        : t('triggerPRPreview');

  return (
    <div
      className={`rounded-lg border p-3 cursor-pointer transition-colors ${
        selected
          ? 'border-primary bg-primary/5'
          : 'border-border bg-card hover:border-primary/40'
      }`}
      onClick={onSelect}
    >
      <div className="flex items-start justify-between">
        <div className="flex-1 min-w-0">
          <div className="truncate font-medium text-foreground">
            {pipeline.name}
          </div>
          <div className="text-xs text-muted-foreground">
            {pipeline.stages.length} {t('columnStages').toLowerCase()} ·{' '}
            {triggerLabel}
          </div>
        </div>
      </div>
      <div className="mt-2 flex gap-1">
        <Button
          size="sm"
          variant="outline"
          onClick={(e) => {
            e.stopPropagation();
            onPromote();
          }}
        >
          <IconRocket className="mr-1.5 size-3.5" />
          {t('promoteAction')}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={(e) => {
            e.stopPropagation();
            onEdit();
          }}
        >
          {t('formActions.update')}
        </Button>
        <Button
          size="icon"
          variant="ghost"
          aria-label={t('formActions.delete')}
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
        >
          <IconTrash className="size-4" />
        </Button>
      </div>
    </div>
  );
}

function PipelineStages({
  pipeline,
  t,
}: {
  pipeline: GitOpsPipeline;
  t: (key: string) => string;
}) {
  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold text-foreground">
        {t('formStages')}
      </h3>
      <div className="space-y-2">
        {pipeline.stages.map((s: GitOpsStage, i: number) => (
          <div
            key={s.id}
            className="rounded-md border border-border bg-card px-3 py-2 text-sm"
          >
            <div className="flex items-center justify-between">
              <div className="font-medium">
                {i + 1}. {s.stageName}
              </div>
              <div className="text-xs text-muted-foreground">
                {s.branch} · {s.deployMode}
                {s.requiresApproval && ' · approval required'}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function DeploymentHistory({
  promotions,
  isLoading,
  t,
}: {
  promotions: import('../types').GitOpsPromotion[];
  isLoading: boolean;
  t: (key: string) => string;
}) {
  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-4">
        <Spinner />
      </div>
    );
  }
  if (promotions.length === 0) {
    return (
      <div>
        <h3 className="mb-2 text-sm font-semibold text-foreground">
          {t('deployments')}
        </h3>
        <p className="text-sm text-muted-foreground">{t('noDeployments')}</p>
      </div>
    );
  }
  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold text-foreground">
        {t('deployments')}
      </h3>
      <div className="space-y-1">
        {promotions.map((p) => (
          <div
            key={p.id}
            className="rounded-md border border-border bg-card px-3 py-2 text-sm"
          >
            <div className="flex items-center justify-between">
              <code className="text-xs">{p.commitSha.slice(0, 8)}</code>
              <span className="text-xs text-muted-foreground">{p.status}</span>
            </div>
            <div className="text-xs text-muted-foreground">
              {p.triggerKind} · {p.startedAt}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
