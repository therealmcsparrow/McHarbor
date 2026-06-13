// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import { IconDownload, IconEye } from "@tabler/icons-react";
import { Badge } from "@resources/components/ui/Badge";
import { Button } from "@resources/components/ui/Button";
import { AppInstallationsSummary } from "./AppInstallations";
import type { AppTemplate } from "../types";

interface AppCardProps {
  app: AppTemplate;
  onInstall: (app: AppTemplate) => void;
  onViewDetail: (app: AppTemplate) => void;
}

function parseImageMeta(image: string) {
  const withoutDigest = image.split("@")[0] ?? image;
  const lastSlash = withoutDigest.lastIndexOf("/");
  const lastColon = withoutDigest.lastIndexOf(":");
  const path = lastColon > lastSlash ? withoutDigest.slice(0, lastColon) : withoutDigest;
  const parts = path.split("/").filter(Boolean);
  const hasRegistry = parts[0]?.includes(".") || parts[0]?.includes(":");
  const registry = hasRegistry ? parts[0] : "docker.io";
  const owner = hasRegistry ? parts[1] : parts.length > 1 ? parts[0] : "library";

  return {
    owner: owner || "library",
    distro: registry || "docker.io",
  };
}

function formatDate(value: string, fallback: string) {
  if (!value) return fallback;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return fallback;
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(date);
}

function countInstallsLast30Days(app: AppTemplate) {
  const cutoff = Date.now() - 30 * 24 * 60 * 60 * 1000;
  return (app.installations ?? []).filter((installation) => {
    const installedAt = new Date(installation.installedAt).getTime();
    return !Number.isNaN(installedAt) && installedAt >= cutoff;
  }).length;
}

export function AppCard({ app, onInstall, onViewDetail }: AppCardProps) {
  const { t } = useTranslation("common");
  const imageMeta = parseImageMeta(app.image);
  const installs30Days = countInstallsLast30Days(app);
  const installsEver = app.installations?.length ?? 0;

  return (
    <div className="group flex h-full flex-col rounded-lg border border-border bg-card transition-colors hover:border-primary/40">
      <div className="m-2 flex flex-1 flex-col gap-3">
        <div className="flex items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted">
            {app.logo ? (
              <img
                src={app.logo}
                alt={app.name}
                className="h-8 w-8 object-contain"
                onError={(e) => {
                  e.currentTarget.style.display = "none";
                }}
              />
            ) : (
              <span className="text-lg font-bold text-muted-foreground">
                {app.name.charAt(0)}
              </span>
            )}
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h3 className="truncate text-sm font-medium text-foreground">
                {app.name}
              </h3>
              {app.version && (
                <span className="shrink-0 text-xs text-muted-foreground">
                  {app.version}
                </span>
              )}
            </div>
            <Badge variant="secondary" className="mt-1 text-xs">
              {app.category}
            </Badge>
          </div>
        </div>

        <p className="line-clamp-2 flex-1 text-xs text-muted-foreground">
          {app.description}
        </p>

        <div className="text-xs font-mono text-muted-foreground truncate">
          {app.image}
        </div>

        <AppInstallationsSummary installations={app.installations ?? []} appName={app.name} />

        <div className="mt-auto flex flex-col gap-2">
          <div className="grid grid-cols-2 gap-3 border-t border-border/60 pt-2">
            <div className="min-w-0 space-y-1 text-left text-[11px] text-muted-foreground">
              <div className="truncate">
                <span className="text-foreground">{t("appStore.latestVersion")}:</span>{" "}
                {app.version || t("appStore.unknown")}
              </div>
              <div className="truncate">
                <span className="text-foreground">{t("appStore.updated")}:</span>{" "}
                {formatDate(app.updatedAt, t("appStore.unknown"))}
              </div>
              <div className="truncate">
                <span className="text-foreground">{t("appStore.owner")}:</span> {imageMeta.owner}
              </div>
              <div className="truncate">
                <span className="text-foreground">{t("appStore.distro")}:</span> {imageMeta.distro}
              </div>
            </div>
            <div className="min-w-0 space-y-1 text-right text-[11px] text-muted-foreground">
              <div className="truncate">
                <span className="text-foreground">{t("appStore.installed30Days")}:</span>{" "}
                {installs30Days}
              </div>
              <div className="truncate">
                <span className="text-foreground">{t("appStore.installedEver")}:</span>{" "}
                {installsEver}
              </div>
            </div>
          </div>
          <div className="flex items-center justify-between gap-2">
            <Button
              size="sm"
              variant="ghost"
              className="h-8 justify-start gap-1 px-2"
              onClick={() => onViewDetail(app)}
            >
              <IconEye className="size-3.5" />
              {t("actions.review")}
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="h-8 justify-end gap-1"
              onClick={() => onInstall(app)}
            >
              <IconDownload className="size-3.5" />
              {t("appStore.install")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
