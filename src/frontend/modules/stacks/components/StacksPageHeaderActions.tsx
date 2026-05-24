// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconArrowUp,
  IconFilterOff,
  IconLayoutGrid,
  IconLayoutList,
  IconPlus,
  IconRefresh,
  IconRotate,
} from "@tabler/icons-react";
import { Button } from "@resources/components/ui/Button";
import { Spinner } from "@resources/components/ui/Spinner";

type StacksPageHeaderActionsProps = {
  t: (key: string, options?: Record<string, unknown>) => string;
  viewMode: "table" | "card";
  setViewMode: (mode: "table" | "card") => void;
  setCreateOpen: (open: boolean) => void;
  checkUpdatesPending: boolean;
  batchRunning: boolean;
  updatesAvailable: number;
  reinstallCount: number;
  pruneRunning: boolean;
  onCheckUpdates: () => void;
  onUpdateAll: () => void;
  onReinstallAll: () => void;
  onPruneUnused: () => void;
};

export function StacksPageHeaderActions({
  t,
  viewMode,
  setViewMode,
  setCreateOpen,
  checkUpdatesPending,
  batchRunning,
  updatesAvailable,
  reinstallCount,
  pruneRunning,
  onCheckUpdates,
  onUpdateAll,
  onReinstallAll,
  onPruneUnused,
}: StacksPageHeaderActionsProps) {
  return (
    <>
      <Button
        variant="outline"
        onClick={onCheckUpdates}
        disabled={checkUpdatesPending || batchRunning}
      >
        {checkUpdatesPending ? (
          <Spinner size="sm" />
        ) : (
          <IconRefresh className="h-4 w-4" />
        )}
        {t("updates.searchForUpdates")}
      </Button>
      {updatesAvailable > 0 && (
        <Button variant="outline" onClick={onUpdateAll} disabled={batchRunning}>
          {batchRunning ? (
            <Spinner size="sm" />
          ) : (
            <IconArrowUp className="h-4 w-4" />
          )}
          {t("updates.updateAll", { count: updatesAvailable })}
        </Button>
      )}
      {reinstallCount > 0 && (
        <Button
          variant="outline"
          onClick={onReinstallAll}
          disabled={batchRunning}
        >
          {batchRunning ? (
            <Spinner size="sm" />
          ) : (
            <IconRotate className="h-4 w-4" />
          )}
          {t("updates.reinstallAll", { count: reinstallCount })}
        </Button>
      )}
      <Button onClick={() => setCreateOpen(true)}>
        <IconPlus className="h-4 w-4" /> {t("deployStack")}
      </Button>
      <Button
        variant="outline"
        onClick={onPruneUnused}
        disabled={batchRunning || pruneRunning}
      >
        {pruneRunning ? (
          <Spinner size="sm" />
        ) : (
          <IconFilterOff className="h-4 w-4" />
        )}
        {t("prune.button")}
      </Button>
      <div className="h-6 w-px bg-border" />
      <div className="flex items-center rounded-lg border border-border">
        <Button
          variant={viewMode === "table" ? "default" : "ghost"}
          size="icon-sm"
          onClick={() => setViewMode("table")}
          aria-label={t("tableView")}
        >
          <IconLayoutList className="h-4 w-4" />
        </Button>
        <Button
          variant={viewMode === "card" ? "default" : "ghost"}
          size="icon-sm"
          onClick={() => setViewMode("card")}
          aria-label={t("cardView")}
        >
          <IconLayoutGrid className="h-4 w-4" />
        </Button>
      </div>
    </>
  );
}
