// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { OperationProgressDialog } from "@resources/components/OperationProgressDialog";
import { DataGrid } from "@resources/components/DataGrid";
import { ConfirmDialog } from "@resources/components/ui/ConfirmDialog";
import { PageHeader } from "@resources/layout/PageHeader";
import { useContainersBulkStats } from "@resources/hooks/useContainersBulkStats";
import {
  useStacks,
  useStackAction,
  useDeleteStack,
  useUpdateStack,
} from "../hooks/useStacks";
import { useCheckStackUpdates } from "../hooks/useStackUpdates";
import { useStackBatchOperations } from "../hooks/useStackBatchOperations";
import { useStacksViewStore } from "../stores/stacks-view";
import { useStackTableColumns } from "../components/StackTableColumns";
import { getStackBatchActions } from "../components/StackBatchActions";
import { StackCardView } from "../components/StackCardView";
import { CreateStackDialog } from "../components/CreateStackDialog";
import { EditStackDialog } from "../components/EditStackDialog";
import { StackLogsDialog } from "../components/StackLogsDialog";
import { TakeOverDialog } from "../components/TakeOverDialog";
import { StacksPageHeaderActions } from "../components/StacksPageHeaderActions";

export default function StacksPage() {
  const { t } = useTranslation("stacks");
  const { t: tc } = useTranslation("common");
  const { data: stacks = [], isLoading } = useStacks();
  const { data: statsMap } = useContainersBulkStats();
  const action = useStackAction();
  const deleteStack = useDeleteStack();
  const updateStack = useUpdateStack();
  const checkUpdates = useCheckStackUpdates();
  const { viewMode, setViewMode } = useStacksViewStore();
  const {
    batchProgress,
    updateResults,
    updateAvailableTargets,
    reinstallTargets,
    getManagedTargets,
    runStackOperation,
  } = useStackBatchOperations(stacks, t);

  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<(typeof stacks)[number] | null>(
    null,
  );
  const [logsTarget, setLogsTarget] = useState<(typeof stacks)[number] | null>(
    null,
  );
  const [removeTarget, setRemoveTarget] = useState<
    (typeof stacks)[number] | null
  >(null);
  const [takeOverTarget, setTakeOverTarget] = useState<
    (typeof stacks)[number] | null
  >(null);
  const [reinstallAllConfirmOpen, setReinstallAllConfirmOpen] = useState(false);

  const handleAction = (name: string, act: string) =>
    action.mutate({ name, action: act });
  const handleDelete = (name: string) => deleteStack.mutate(name);
  const updatesAvailable = updateAvailableTargets.length;

  const columns = useStackTableColumns({
    onEdit: setEditTarget,
    onLogs: setLogsTarget,
    onRemove: setRemoveTarget,
    onTakeOver: setTakeOverTarget,
    onAction: handleAction,
    updateResults,
  });

  const batchActions = getStackBatchActions({
    tc,
    onAction: handleAction,
    onDelete: handleDelete,
    onUpdateSelected: (rows) =>
      runStackOperation("update", getManagedTargets(rows)),
    onReinstallSelected: (rows) =>
      runStackOperation("reinstall", getManagedTargets(rows)),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("title")}
        description={t("description", { count: stacks.length })}
        actions={
          <StacksPageHeaderActions
            t={t}
            viewMode={viewMode}
            setViewMode={setViewMode}
            setCreateOpen={setCreateOpen}
            checkUpdatesPending={checkUpdates.isPending}
            batchRunning={batchProgress.isRunning}
            updatesAvailable={updatesAvailable}
            reinstallCount={reinstallTargets.length}
            onCheckUpdates={() => checkUpdates.mutate(undefined)}
            onUpdateAll={() =>
              runStackOperation("update", updateAvailableTargets)
            }
            onReinstallAll={() => setReinstallAllConfirmOpen(true)}
          />
        }
      />

      {viewMode === "table" ? (
        <DataGrid
          data={stacks}
          columns={columns}
          searchKey="name"
          searchPlaceholder={t("searchPlaceholder")}
          loading={isLoading}
          emptyMessage={t("emptyMessage")}
          selectable
          batchActions={batchActions}
          getRowId={(row) => row.id}
        />
      ) : (
        <StackCardView
          stacks={stacks}
          isLoading={isLoading}
          statsMap={statsMap}
          onAction={handleAction}
          onEdit={setEditTarget}
          onLogs={setLogsTarget}
          onRemove={setRemoveTarget}
          onTakeOver={setTakeOverTarget}
        />
      )}

      <CreateStackDialog open={createOpen} onOpenChange={setCreateOpen} />

      <EditStackDialog
        stack={editTarget}
        onClose={() => setEditTarget(null)}
        onSave={(compose) => {
          if (!editTarget) return;
          updateStack.mutate(
            { name: editTarget.name, compose },
            { onSuccess: () => setEditTarget(null) },
          );
        }}
        saving={updateStack.isPending}
      />

      <StackLogsDialog stack={logsTarget} onClose={() => setLogsTarget(null)} />

      <TakeOverDialog
        open={takeOverTarget !== null}
        onOpenChange={(open) => !open && setTakeOverTarget(null)}
        stackName={takeOverTarget?.name}
      />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(open) => !open && setRemoveTarget(null)}
        title={t("confirm.removeTitle")}
        description={t("confirm.removeDescription", {
          name: removeTarget?.name ?? "",
        })}
        confirmLabel={t("confirm.removeLabel")}
        onConfirm={() => {
          if (removeTarget) deleteStack.mutate(removeTarget.name);
          setRemoveTarget(null);
        }}
        loading={deleteStack.isPending}
      />

      <ConfirmDialog
        open={reinstallAllConfirmOpen}
        onOpenChange={setReinstallAllConfirmOpen}
        title={t("updates.confirm.reinstallAllTitle")}
        description={t("updates.confirm.reinstallAllDescription", {
          count: reinstallTargets.length,
        })}
        confirmLabel={t("updates.progress.reinstallAction")}
        onConfirm={() => {
          runStackOperation("reinstall", reinstallTargets);
          setReinstallAllConfirmOpen(false);
        }}
        loading={batchProgress.isRunning}
      />

      <OperationProgressDialog
        state={batchProgress.dialogState}
        onClose={batchProgress.closeDialog}
      />
    </div>
  );
}
