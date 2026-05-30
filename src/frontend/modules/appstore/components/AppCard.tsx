// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import { IconCheck, IconDownload, IconEye } from "@tabler/icons-react";
import { Badge } from "@resources/components/ui/Badge";
import { Button } from "@resources/components/ui/Button";
import { AppInstallationsSummary } from "./AppInstallations";
import type { AppTemplate } from "../types";

interface AppCardProps {
  app: AppTemplate;
  onInstall: (app: AppTemplate) => void;
  onViewDetail: (app: AppTemplate) => void;
}

export function AppCard({ app, onInstall, onViewDetail }: AppCardProps) {
  const { t } = useTranslation("common");
  return (
    <div className="group flex flex-col rounded-lg border border-border bg-card transition-colors hover:border-primary/40">
      <div className="m-2 flex flex-col gap-3">
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

        <AppInstallationsSummary installations={app.installations ?? []} />

        <div className="flex flex-col gap-2">
          <div className="flex items-center">
            {app.installed ? (
              <Badge variant="success" className="gap-1">
                <IconCheck className="size-3" />
                {t("appStore.installed")}
              </Badge>
            ) : (
              <span className="text-xs text-muted-foreground">
                {t("appStore.availableForInstall")}
              </span>
            )}
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
