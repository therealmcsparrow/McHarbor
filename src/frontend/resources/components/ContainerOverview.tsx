// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { useQueries } from "@tanstack/react-query";
import { Command } from "cmdk";
import { IconBox, IconLoader2 } from "@tabler/icons-react";
import { api } from "@core/api/client";
import type { ContainerInfo, ContainerState } from "@core/types/docker";
import { useEnvironmentStore } from "@resources/stores/environment";

type ContainerWithEnv = ContainerInfo & { envId: string; envName: string };

const STATE_DOT: Record<ContainerState, string> = {
  running: "bg-emerald-500",
  paused: "bg-amber-500",
  restarting: "bg-amber-500",
  created: "bg-zinc-400",
  exited: "bg-red-500",
  removing: "bg-red-400",
  dead: "bg-red-600",
};

function containerName(c: ContainerInfo): string {
  const name = c.Names?.[0] ?? c.Id.slice(0, 12);
  return name.startsWith("/") ? name.slice(1) : name;
}

type ContainerOverviewProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ContainerOverview({
  open,
  onOpenChange,
}: ContainerOverviewProps) {
  const { t } = useTranslation("containers");
  const { t: tc } = useTranslation("common");
  const navigate = useNavigate();
  const environments = useEnvironmentStore((s) => s.environments);

  const dockerEnvs = useMemo(
    () => environments.filter((e) => e.orchestratorType === "docker"),
    [environments],
  );

  const queries = useQueries({
    queries: dockerEnvs.map((env) => ({
      queryKey: ["containers-overview", env.id],
      queryFn: () =>
        api
          .get<ContainerInfo[]>("/containers", { all: "true", env: env.id })
          .then((r) =>
            (r.data ?? []).map((c) => ({
              ...c,
              envId: env.id,
              envName: env.name,
            })),
          ),
      refetchInterval: open ? 10_000 : false,
      staleTime: 5_000,
      enabled: open,
    })),
  });

  const isLoading = queries.some((q) => q.isLoading);

  const envGroups = useMemo(() => {
    const groups: {
      envId: string;
      envName: string;
      containers: ContainerWithEnv[];
    }[] = [];
    for (let i = 0; i < dockerEnvs.length; i++) {
      const env = dockerEnvs[i];
      const containers = queries[i]?.data ?? [];
      if (env) {
        groups.push({ envId: env.id, envName: env.name, containers });
      }
    }
    return groups;
  }, [dockerEnvs, queries]);

  const totalCount = envGroups.reduce((sum, g) => sum + g.containers.length, 0);
  const runningCount = envGroups.reduce(
    (sum, g) => sum + g.containers.filter((c) => c.State === "running").length,
    0,
  );

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && open) {
        e.preventDefault();
        onOpenChange(false);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onOpenChange]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-[12vh]"
      onClick={() => onOpenChange(false)}
    >
      <div className="fixed inset-0 bg-background/80 backdrop-blur-sm" />
      <div
        className="relative z-10 w-full max-w-2xl rounded-lg border border-border bg-popover shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <Command className="flex flex-col" shouldFilter>
          <div className="flex items-center border-b border-border">
            <IconBox className="ml-4 h-4 w-4 shrink-0 text-muted-foreground" />
            <Command.Input
              placeholder={t("searchPlaceholder")}
              className="w-full bg-transparent px-3 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
              autoFocus
            />
            {isLoading && (
              <IconLoader2 className="mr-4 h-4 w-4 shrink-0 animate-spin text-muted-foreground" />
            )}
          </div>

          {/* Summary bar */}
          <div className="flex items-center gap-3 border-b border-border px-4 py-2 text-xs text-muted-foreground">
            <span>{tc("containerOverview.total", { count: totalCount })}</span>
            <span className="text-emerald-500">
              {tc("containerOverview.running", { count: runningCount })}
            </span>
            <span className="ml-auto">
              {tc("containerOverview.environments", {
                count: dockerEnvs.length,
              })}
            </span>
          </div>

          <Command.List className="max-h-[50vh] overflow-y-auto p-2">
            <Command.Empty className="px-4 py-8 text-center text-sm text-muted-foreground">
              {t("emptyMessage")}
            </Command.Empty>

            {envGroups.map((group) => (
              <Command.Group
                key={group.envId}
                heading={`${group.envName} (${group.containers.length})`}
                className="px-2 py-1.5 text-xs font-medium uppercase tracking-wider text-muted-foreground"
              >
                {group.containers.map((c) => {
                  const name = containerName(c);
                  const dotClass = STATE_DOT[c.State] ?? "bg-zinc-400";
                  return (
                    <Command.Item
                      key={`${group.envId}-${c.Id}`}
                      value={`${name} ${c.Image} ${c.State} ${group.envName}`}
                      onSelect={() => {
                        navigate(`/containers/${c.Id}?env=${group.envId}`);
                        onOpenChange(false);
                      }}
                      className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                    >
                      <span
                        className={`h-2 w-2 shrink-0 rounded-full ${dotClass}`}
                      />
                      <span className="min-w-0 truncate font-medium">
                        {name}
                      </span>
                      <span className="min-w-0 truncate text-xs text-muted-foreground">
                        {c.Image}
                      </span>
                      <span className="ml-auto shrink-0 text-xs text-muted-foreground">
                        {c.Status}
                      </span>
                    </Command.Item>
                  );
                })}
              </Command.Group>
            ))}
          </Command.List>

          {/* Footer hint */}
          <div className="flex items-center gap-4 border-t border-border px-4 py-2 text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                ↑↓
              </kbd>
              {tc("containerOverview.navigate")}
            </span>
            <span className="flex items-center gap-1.5">
              <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                ↵
              </kbd>
              {tc("containerOverview.open")}
            </span>
            <span className="flex items-center gap-1.5">
              <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                esc
              </kbd>
              {tc("containerOverview.close")}
            </span>
          </div>
        </Command>
      </div>
    </div>
  );
}
