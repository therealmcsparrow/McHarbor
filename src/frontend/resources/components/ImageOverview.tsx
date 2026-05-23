// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { useQueries } from "@tanstack/react-query";
import { Command } from "cmdk";
import { IconPhoto, IconLoader2 } from "@tabler/icons-react";
import { api } from "@core/api/client";
import type { ImageInfo } from "@core/types/docker";
import { useEnvironmentStore } from "@resources/stores/environment";
import { formatBytes } from "@resources/utils/format";

type ImageWithEnv = ImageInfo & { envId: string; envName: string };

function imageLabel(img: ImageInfo): string {
  const firstTag = img.RepoTags?.[0];
  if (firstTag && firstTag !== "<none>:<none>") {
    return firstTag;
  }
  return img.Id.replace("sha256:", "").slice(0, 12);
}

type ImageOverviewProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ImageOverview({ open, onOpenChange }: ImageOverviewProps) {
  const { t } = useTranslation("images");
  const { t: tc } = useTranslation("common");
  const navigate = useNavigate();
  const environments = useEnvironmentStore((s) => s.environments);

  const dockerEnvs = useMemo(
    () => environments.filter((e) => e.orchestratorType === "docker"),
    [environments],
  );

  const queries = useQueries({
    queries: dockerEnvs.map((env) => ({
      queryKey: ["images-overview", env.id],
      queryFn: () =>
        api
          .get<ImageInfo[]>("/images", { env: env.id })
          .then((r) =>
            (r.data ?? []).map((img) => ({
              ...img,
              envId: env.id,
              envName: env.name,
            })),
          ),
      refetchInterval: open ? 30_000 : false,
      staleTime: 10_000,
      enabled: open,
    })),
  });

  const isLoading = queries.some((q) => q.isLoading);

  const envGroups = useMemo(() => {
    const groups: { envId: string; envName: string; images: ImageWithEnv[] }[] =
      [];
    for (let i = 0; i < dockerEnvs.length; i++) {
      const env = dockerEnvs[i];
      const images = queries[i]?.data ?? [];
      if (env) {
        groups.push({ envId: env.id, envName: env.name, images });
      }
    }
    return groups;
  }, [dockerEnvs, queries]);

  const totalCount = envGroups.reduce((sum, g) => sum + g.images.length, 0);
  const totalSize = envGroups.reduce(
    (sum, g) => sum + g.images.reduce((s, img) => s + img.Size, 0),
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
            <IconPhoto className="ml-4 h-4 w-4 shrink-0 text-muted-foreground" />
            <Command.Input
              placeholder={t("searchPlaceholder")}
              className="w-full bg-transparent px-3 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
              autoFocus
            />
            {isLoading && (
              <IconLoader2 className="mr-4 h-4 w-4 shrink-0 animate-spin text-muted-foreground" />
            )}
          </div>

          <div className="flex items-center gap-3 border-b border-border px-4 py-2 text-xs text-muted-foreground">
            <span>{tc("imageOverview.total", { count: totalCount })}</span>
            <span>
              {tc("imageOverview.totalSize", { size: formatBytes(totalSize) })}
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
                heading={`${group.envName} (${group.images.length})`}
                className="px-2 py-1.5 text-xs font-medium uppercase tracking-wider text-muted-foreground"
              >
                {group.images.map((img) => {
                  const label = imageLabel(img);
                  const inUse = img.Containers > 0;
                  return (
                    <Command.Item
                      key={`${group.envId}-${img.Id}`}
                      value={`${label} ${group.envName}`}
                      onSelect={() => {
                        navigate(
                          `/images/${encodeURIComponent(img.Id)}?env=${group.envId}`,
                        );
                        onOpenChange(false);
                      }}
                      className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                    >
                      <span
                        className={`h-2 w-2 shrink-0 rounded-full ${inUse ? "bg-emerald-500" : "bg-zinc-400"}`}
                      />
                      <span className="min-w-0 truncate font-medium">
                        {label}
                      </span>
                      {img.RepoTags && img.RepoTags.length > 1 && (
                        <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                          +{img.RepoTags.length - 1}
                        </span>
                      )}
                      <span className="ml-auto shrink-0 text-xs tabular-nums text-muted-foreground">
                        {formatBytes(img.Size)}
                      </span>
                    </Command.Item>
                  );
                })}
              </Command.Group>
            ))}
          </Command.List>

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
