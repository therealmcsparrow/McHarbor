// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconHistory } from "@tabler/icons-react";
import { Button } from "@resources/components/ui/Button";
import { ConfirmDialog } from "@resources/components/ui/ConfirmDialog";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@resources/components/ui/Tabs";
import { EditPlanDialog } from "../components/EditPlanDialog";
import { RestoreRunDialog } from "../components/RestoreRunDialog";
import { PageHeader } from "@resources/layout/PageHeader";
import { useEnvironmentStore } from "@resources/stores/environment";
import { useNavigate } from "react-router";
import {
  containerBackupDownloadUrl,
  useAllContainerBackupPlans,
  useAllContainerBackupRuns,
  useDeleteContainerBackupPlan,
  useDeleteContainerBackupRun,
  useRelinkAllContainerBackups,
  type ContainerBackupPlan,
  type ContainerBackupRun,
} from "../hooks/useContainerBackups";
import {
  BackupPlansTable,
  BackupRunsTable,
  PlanBulkToolbar,
  RunsBulkToolbar,
} from "./backups-page-tables";
import BackupLogsTab from "../components/BackupLogsTab";

export default function BackupsPage() {
  const { t } = useTranslation("containers");
  const { t: tc } = useTranslation("common");
  const navigate = useNavigate();
  const envId = useEnvironmentStore((s) => s.currentId);
  const setCurrentId = useEnvironmentStore((s) => s.setCurrentId);
  const environments = useEnvironmentStore((s) => s.environments);
  const [tab, setTab] = useState<"runs" | "plans" | "logs">("runs");
  const runsQuery = useAllContainerBackupRuns(envId);
  const plansQuery = useAllContainerBackupPlans(envId);
  const deletePlan = useDeleteContainerBackupPlan();
  const deleteRun = useDeleteContainerBackupRun();
  const relinkAll = useRelinkAllContainerBackups();
  const [editTarget, setEditTarget] = useState<ContainerBackupPlan | null>(
    null,
  );
  const [deleteTarget, setDeleteTarget] =
    useState<ContainerBackupPlan | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<ContainerBackupRun | null>(
    null,
  );
  const [deleteRunTarget, setDeleteRunTarget] =
    useState<ContainerBackupRun | null>(null);
  const [bulkDeleteRunTargets, setBulkDeleteRunTargets] = useState<
    ContainerBackupRun[]
  >([]);
  const [bulkDeleteTargets, setBulkDeleteTargets] = useState<
    ContainerBackupPlan[]
  >([]);
  const [selectedPlanIds, setSelectedPlanIds] = useState<Set<string>>(
    () => new Set(),
  );
  const [selectedRunIds, setSelectedRunIds] = useState<Set<string>>(
    () => new Set(),
  );

  const envNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const env of environments) {
      map.set(env.id, env.name);
    }
    return map;
  }, [environments]);

  // Reset selection when the plan list changes (env switch, query
  // refresh, deletes). Without this, a stale id can linger and the
  // bulk-action toolbar's count drifts from reality.
  useEffect(() => {
    setSelectedPlanIds((current) => reconcileSelection(current, plansQuery.data.map((p) => p.id)));
    setSelectedRunIds((current) => reconcileSelection(current, runsQuery.data.map((r) => r.id)));
  }, [plansQuery.data, runsQuery.data]);

  const selectedPlans = useMemo(
    () => plansQuery.data.filter((plan) => selectedPlanIds.has(plan.id)),
    [plansQuery.data, selectedPlanIds],
  );
  const selectedRuns = useMemo(
    () => runsQuery.data.filter((run) => selectedRunIds.has(run.id)),
    [runsQuery.data, selectedRunIds],
  );

  function reconcileSelection(current: Set<string>, liveIds: string[]) {
    const live = new Set(liveIds);
    const next = new Set<string>();
    for (const id of current) {
      if (live.has(id)) next.add(id);
    }
    return next.size === current.size ? current : next;
  }

  function togglePlanSelected(planId: string) {
    setSelectedPlanIds((current) => {
      const next = new Set(current);
      if (next.has(planId)) next.delete(planId);
      else next.add(planId);
      return next;
    });
  }

  function toggleAllPlansSelected() {
    setSelectedPlanIds((current) => {
      if (current.size === plansQuery.data.length) return new Set();
      return new Set(plansQuery.data.map((plan) => plan.id));
    });
  }

  // openContainer switches the global environment to the run /
  // plan's owning environment, then navigates to the container
  // detail page. We can't just link to `/containers/{id}`
  // because the ContainerDetailPage reads the active env from
  // useEnvironmentStore (via the `/containers/{id}` route + the
  // `useContainer` hook). On the Backups page with "All
  // environments" selected, the global env may not match the
  // row's env — switching first ensures the detail page queries
  // the right orchestrator.
  function openContainer(environmentId: string, containerId: string) {
    if (environmentId && environmentId !== envId) {
      setCurrentId(environmentId);
    }
    navigate(`/containers/${containerId}`);
  }

  function triggerBrowserDownload(run: ContainerBackupRun) {
    const url = containerBackupDownloadUrl(run.id);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${run.containerName || run.containerId.slice(0, 12)}-${run.id}.mcharbor.tar`;
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
  }

  function bulkDownloadRuns(runs: ContainerBackupRun[]) {
    for (const run of runs) {
      triggerBrowserDownload(run);
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <PageHeader
        title={tc("nav.backups")}
        description={
          environments.find((e) => e.id === envId)?.name
        }
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => relinkAll.mutate()}
            disabled={relinkAll.isPending}
            data-testid="backups-relink-all"
          >
            <IconHistory className="size-4" aria-hidden="true" />
            {t("backups.relinkAll")}
          </Button>
        }
      />

      <div className="flex min-h-0 flex-1 flex-col">
        <Tabs
          value={tab}
          onValueChange={(v) => setTab(v as "runs" | "plans" | "logs")}
          className="flex min-h-0 flex-1 flex-col"
        >
          <TabsList>
            <TabsTrigger value="runs">{t("backups.tabRuns")}</TabsTrigger>
            <TabsTrigger value="plans">{t("backups.tabPlans")}</TabsTrigger>
            <TabsTrigger value="logs">{t("backups.logs.title")}</TabsTrigger>
          </TabsList>
          <TabsContent value="runs" className="min-h-0 flex-1 space-y-3 flex flex-col">
            <RunsBulkToolbar
              selected={selectedRuns}
              total={runsQuery.data.length}
              onClearSelection={() => setSelectedRunIds(new Set())}
              onBulkDownload={() => bulkDownloadRuns(selectedRuns)}
              onBulkDelete={() => setBulkDeleteRunTargets([...selectedRuns])}
              deleting={deleteRun.isPending}
            />
            <BackupRunsTable
              runs={runsQuery.data}
              loading={runsQuery.isLoading}
              envNameById={envNameById}
              selectedIds={selectedRunIds}
              onToggleSelected={(id) =>
                setSelectedRunIds((current) => {
                  const next = new Set(current);
                  if (next.has(id)) next.delete(id);
                  else next.add(id);
                  return next;
                })
              }
              onToggleAll={() =>
                setSelectedRunIds((current) => {
                  if (current.size === runsQuery.data.length) return new Set();
                  return new Set(runsQuery.data.map((run) => run.id));
                })
              }
              onRestore={(run) => setRestoreTarget(run)}
              onDelete={(run) => setDeleteRunTarget(run)}
              onDownload={triggerBrowserDownload}
              onOpenContainer={openContainer}
            />
          </TabsContent>
          <TabsContent value="plans" className="min-h-0 flex-1 space-y-3 flex flex-col">
            <PlanBulkToolbar
              selected={selectedPlans}
              total={plansQuery.data.length}
              onClearSelection={() => setSelectedPlanIds(new Set())}
              onBulkDelete={() => setBulkDeleteTargets([...selectedPlans])}
              deleting={deletePlan.isPending}
            />
            <BackupPlansTable
              plans={plansQuery.data}
              loading={plansQuery.isLoading}
              envNameById={envNameById}
              selectedIds={selectedPlanIds}
              onToggleSelected={togglePlanSelected}
              onToggleAll={toggleAllPlansSelected}
              onEdit={(plan) => setEditTarget(plan)}
              onDelete={(plan) => setDeleteTarget(plan)}
              onOpenContainer={openContainer}
            />
          </TabsContent>
          <TabsContent value="logs" className="space-y-3">
            <BackupLogsTab />
          </TabsContent>
        </Tabs>
      </div>

      <RestoreRunDialog
        run={restoreTarget}
        containerId={restoreTarget?.containerId ?? ""}
        onClose={() => setRestoreTarget(null)}
      />
      {deleteRunTarget && (
        <ConfirmDialog
          open={!!deleteRunTarget}
          onOpenChange={(open) => {
            if (!open) setDeleteRunTarget(null);
          }}
          title={t("backups.deleteRunTitle")}
          description={t("backups.deleteRunDescription")}
          confirmLabel={t("backups.deleteRun")}
          loading={deleteRun.isPending}
          onConfirm={() => {
            deleteRun.mutate(deleteRunTarget.id, {
              onSuccess: () => setDeleteRunTarget(null),
            });
          }}
        />
      )}
      {bulkDeleteRunTargets.length > 0 && (
        <ConfirmDialog
          open={bulkDeleteRunTargets.length > 0}
          onOpenChange={(open) => {
            if (!open) setBulkDeleteRunTargets([]);
          }}
          title={t("backups.bulkRunsDeleteTitle")}
          description={t("backups.bulkRunsDeleteDescription", {
            count: bulkDeleteRunTargets.length,
          })}
          confirmLabel={t("backups.bulkRunsDeleteConfirm", {
            count: bulkDeleteRunTargets.length,
          })}
          loading={deleteRun.isPending}
          onConfirm={() => {
            const ids = bulkDeleteRunTargets.map((run) => run.id);
            setBulkDeleteRunTargets([]);
            setSelectedRunIds(new Set());
            const run = (index: number) => {
              if (index >= ids.length) return;
              const id = ids[index];
              if (!id) return;
              deleteRun.mutate(id, {
                onSuccess: () => run(index + 1),
              });
            };
            run(0);
          }}
        />
      )}
      <EditPlanDialog
        plan={editTarget}
        onClose={() => setEditTarget(null)}
      />
      {deleteTarget && (
        <ConfirmDialog
          open={!!deleteTarget}
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("backups.deleteScheduleTitle")}
          description={t("backups.deleteScheduleDescription")}
          confirmLabel={t("backups.deleteSchedule")}
          loading={deletePlan.isPending}
          onConfirm={() => {
            deletePlan.mutate(deleteTarget.id, {
              onSuccess: () => setDeleteTarget(null),
            });
          }}
        />
      )}
      {bulkDeleteTargets.length > 0 && (
        <ConfirmDialog
          open={bulkDeleteTargets.length > 0}
          onOpenChange={(open) => {
            if (!open) setBulkDeleteTargets([]);
          }}
          title={t("backups.bulkDeleteTitle")}
          description={t("backups.bulkDeleteDescription", {
            count: bulkDeleteTargets.length,
          })}
          confirmLabel={t("backups.bulkDeleteConfirm", {
            count: bulkDeleteTargets.length,
          })}
          loading={deletePlan.isPending}
          onConfirm={() => {
            // Delete sequentially so a single failure surfaces a
            // toast and leaves the rest deletable. The mutation
            // hook already invalidates the plans query on each
            // success, so the table refreshes incrementally.
            const ids = bulkDeleteTargets.map((plan) => plan.id);
            setBulkDeleteTargets([]);
            setSelectedPlanIds(new Set());
            const run = (index: number) => {
              if (index >= ids.length) return;
              const id = ids[index];
              if (!id) return;
              deletePlan.mutate(id, {
                onSuccess: () => run(index + 1),
              });
            };
            run(0);
          }}
        />
      )}
    </div>
  );
}
