// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  useDeleteDeployment,
  useDeletePod,
  useRestartDeployment,
  useScaleDeployment,
  type ResourceRef,
} from '../hooks/useKubernetesActions';
import type { KubernetesSummary } from '../types';
import type { ActionsKind } from './KubernetesActionsCell';
import type { KubernetesTableActions } from './KubernetesTables';
import { PodLogsDialog } from './PodLogsDialog';
import { ScaleDeploymentDialog } from './ScaleDeploymentDialog';

type DeleteTarget = { ref: ResourceRef; kind: ActionsKind };

export type KubernetesActionDialogsProps = {
  scaleTarget: ResourceRef | null;
  setScaleTarget: (ref: ResourceRef | null) => void;
  deleteTarget: DeleteTarget | null;
  setDeleteTarget: (target: DeleteTarget | null) => void;
  logsTarget: ResourceRef | null;
  setLogsTarget: (ref: ResourceRef | null) => void;
  currentReplicas: number;
  /**
   * Fired once a single-resource delete has actually succeeded. The detail
   * view uses it to leave a page whose subject no longer exists — otherwise
   * the reader is left staring at a "detail unavailable" card and has to work
   * out for themselves that it means the delete worked.
   */
  onDeleted?: () => void;
};

/**
 * Owns the state behind the row actions.
 *
 * The page is already large and the tables only need callbacks, so the dialog
 * bookkeeping lives here: the hook hands the tables their `actions` object and
 * hands the page an opaque props bag for `<KubernetesActionDialogs>`.
 */
export function useKubernetesActionDialogs(summary: KubernetesSummary | undefined, onDeleted?: () => void) {
  const [scaleTarget, setScaleTarget] = useState<ResourceRef | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);
  const [logsTarget, setLogsTarget] = useState<ResourceRef | null>(null);

  const restart = useRestartDeployment();
  const deletePod = useDeletePod();
  const deleteDeployment = useDeleteDeployment();

  // Seed the scale field from what the cluster currently reports rather than
  // from 1 — the reader is adjusting a number, not inventing one.
  const currentReplicas = useMemo(() => {
    if (!scaleTarget || !summary) return 1;
    const match = summary.deployments.find(
      (row) => row.namespace === scaleTarget.namespace && row.name === scaleTarget.name,
    );
    return typeof match?.replicas === 'number' ? match.replicas : 1;
  }, [scaleTarget, summary]);

  const actions: KubernetesTableActions = useMemo(
    () => ({
      onScale: setScaleTarget,
      onRestart: (ref) => restart.mutate(ref),
      onDelete: (ref, kind) => setDeleteTarget({ ref, kind }),
      onLogs: setLogsTarget,
      // The bulk bar has already confirmed once for the whole selection, so
      // these fire straight away rather than re-opening the single-resource
      // dialog per row. There is no bulk endpoint behind them: the API is
      // per-resource, and one invalidation per settled mutation is cheap
      // because every tab reads the same cached summary.
      onDeleteMany: (refs, kind) => {
        const mutation = kind === 'pods' ? deletePod : deleteDeployment;
        for (const ref of refs) mutation.mutate(ref);
      },
      onRestartMany: (refs) => {
        for (const ref of refs) restart.mutate(ref);
      },
    }),
    [deleteDeployment, deletePod, restart],
  );

  const dialogProps: KubernetesActionDialogsProps = {
    scaleTarget,
    setScaleTarget,
    deleteTarget,
    setDeleteTarget,
    logsTarget,
    setLogsTarget,
    currentReplicas,
    onDeleted,
  };

  return { actions, dialogProps };
}

export function KubernetesActionDialogs({
  scaleTarget,
  setScaleTarget,
  deleteTarget,
  setDeleteTarget,
  logsTarget,
  setLogsTarget,
  currentReplicas,
  onDeleted,
}: KubernetesActionDialogsProps) {
  const { t } = useTranslation('kubernetes');
  const scale = useScaleDeployment();
  const deleteDeployment = useDeleteDeployment();
  const deletePod = useDeletePod();

  const confirmDelete = useCallback(() => {
    if (!deleteTarget) return;
    const mutation = deleteTarget.kind === 'pods' ? deletePod : deleteDeployment;
    mutation.mutate(deleteTarget.ref, {
      onSuccess: () => {
        setDeleteTarget(null);
        onDeleted?.();
      },
    });
  }, [deleteTarget, deletePod, deleteDeployment, onDeleted, setDeleteTarget]);

  const deleting = deletePod.isPending || deleteDeployment.isPending;

  return (
    <>
      <ScaleDeploymentDialog
        target={scaleTarget}
        currentReplicas={currentReplicas}
        loading={scale.isPending}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setScaleTarget(null);
        }}
        onConfirm={(replicas) => {
          if (!scaleTarget) return;
          scale.mutate({ ...scaleTarget, replicas }, { onSuccess: () => setScaleTarget(null) });
        }}
      />

      <PodLogsDialog
        target={logsTarget}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setLogsTarget(null);
        }}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setDeleteTarget(null);
        }}
        title={deleteTarget?.kind === 'pods' ? t('confirm.deletePodTitle') : t('confirm.deleteDeploymentTitle')}
        description={
          deleteTarget?.kind === 'pods'
            ? t('confirm.deletePodDescription', {
                name: deleteTarget?.ref.name ?? '',
                namespace: deleteTarget?.ref.namespace ?? '',
              })
            : t('confirm.deleteDeploymentDescription', {
                name: deleteTarget?.ref.name ?? '',
                namespace: deleteTarget?.ref.namespace ?? '',
              })
        }
        confirmLabel={t('actions.delete')}
        loading={deleting}
        onConfirm={confirmDelete}
      />
    </>
  );
}
