// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconDownload, IconRefresh } from "@tabler/icons-react";
import { Badge } from "@resources/components/ui/Badge";
import { Button } from "@resources/components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@resources/components/ui/Card";
import { ConfirmDialog } from "@resources/components/ui/ConfirmDialog";
import { Spinner } from "@resources/components/ui/Spinner";
import { LogViewer } from "@resources/components/LogViewer";
import { useApplyOSUpdates, useOSUpdateCheck } from "../hooks/useSystemOS";
import type { OSUpdateApplyResult } from "../types";

export function SystemOSUpdatesTab() {
  const { t } = useTranslation("system");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [applyResult, setApplyResult] = useState<OSUpdateApplyResult | null>(
    null,
  );
  const check = useOSUpdateCheck();
  const apply = useApplyOSUpdates();

  const outputLines = useMemo(() => {
    const output = applyResult?.output ?? check.data?.output ?? "";
    return output.split("\n").filter((line) => line.trim().length > 0);
  }, [applyResult?.output, check.data?.output]);

  async function handleApply() {
    try {
      const result = await apply.mutateAsync();
      setApplyResult(result);
      setConfirmOpen(false);
    } catch {
      // Toast handling is centralized by the query client mutation metadata.
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <CardTitle className="text-base">{t("updates.title")}</CardTitle>
          <CardDescription>{t("updates.description")}</CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {check.data && (
            <Badge variant={check.data.available ? "warning" : "success"}>
              {check.data.available
                ? t("updates.available")
                : t("updates.current")}
            </Badge>
          )}
          {applyResult && (
            <Badge variant={applyResult.success ? "success" : "destructive"}>
              {applyResult.success
                ? t("updates.applySucceeded")
                : t("updates.applyFailed")}
            </Badge>
          )}
          <Button
            variant="outline"
            onClick={() => void check.refetch()}
            disabled={check.isFetching}
          >
            <IconRefresh className="size-4" />
            {t("updates.check")}
          </Button>
          <Button
            variant="destructive"
            onClick={() => setConfirmOpen(true)}
            disabled={apply.isPending}
          >
            <IconDownload className="size-4" />
            {t("updates.apply")}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 md:grid-cols-3">
          <InfoTile
            label={t("updates.manager")}
            value={
              applyResult?.manager ??
              check.data?.manager ??
              t("updates.unknown")
            }
          />
          <InfoTile
            label={t("updates.lastChecked")}
            value={
              check.data?.checkedAt
                ? new Date(check.data.checkedAt).toLocaleString()
                : t("updates.notChecked")
            }
          />
          <InfoTile
            label={t("updates.exitCode")}
            value={
              applyResult ? String(applyResult.exitCode) : t("updates.notRun")
            }
          />
        </div>

        {check.isFetching && (
          <div className="flex h-40 items-center justify-center">
            <Spinner />
          </div>
        )}
        {check.isError && (
          <div className="rounded-lg border border-border bg-muted/20 px-4 py-3 text-sm text-muted-foreground">
            {t("updates.checkFailed")}
          </div>
        )}
        <LogViewer
          lines={outputLines}
          emptyMessage={t("updates.empty")}
          className="h-[420px]"
        />
      </CardContent>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("updates.confirmTitle")}
        description={t("updates.confirmDescription")}
        confirmLabel={t("updates.apply")}
        loading={apply.isPending}
        onConfirm={() => {
          void handleApply();
        }}
      />
    </Card>
  );
}

type InfoTileProps = {
  label: string;
  value: string;
};

function InfoTile({ label, value }: InfoTileProps) {
  return (
    <div className="rounded-lg border border-border bg-muted/20 px-4 py-3">
      <div className="text-xs font-medium uppercase text-muted-foreground">
        {label}
      </div>
      <div className="mt-1 truncate text-sm font-medium text-foreground">
        {value}
      </div>
    </div>
  );
}
