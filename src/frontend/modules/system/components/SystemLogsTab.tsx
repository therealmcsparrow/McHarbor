// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@resources/components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@resources/components/ui/Card";
import { Input } from "@resources/components/ui/Input";
import { Select } from "@resources/components/ui/Select";
import { Spinner } from "@resources/components/ui/Spinner";
import { LogViewer } from "@resources/components/LogViewer";
import { useOSLogs } from "../hooks/useSystemOS";
import type { OSLogSource } from "../types";

export function SystemLogsTab() {
  const { t } = useTranslation("system");
  const [source, setSource] = useState<OSLogSource>("system");
  const [tail, setTail] = useState(200);
  const { data, isLoading, isFetching, isError, refetch } = useOSLogs(
    source,
    tail,
  );

  const sourceOptions = useMemo(
    () => [
      { value: "system", label: t("logs.sources.system") },
      { value: "kernel", label: t("logs.sources.kernel") },
      { value: "auth", label: t("logs.sources.auth") },
      { value: "docker", label: t("logs.sources.docker") },
    ],
    [t],
  );

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <CardTitle className="text-base">{t("logs.title")}</CardTitle>
          <CardDescription>{t("logs.description")}</CardDescription>
        </div>
        <div className="grid gap-2 sm:grid-cols-[180px_110px_auto]">
          <Select
            value={source}
            onChange={(value) => setSource(value as OSLogSource)}
            options={sourceOptions}
            searchable={false}
            ariaLabel={t("logs.sourceLabel")}
          />
          <Input
            type="number"
            min={1}
            max={1000}
            value={tail}
            aria-label={t("logs.tailLabel")}
            onChange={(event) => {
              const next = Number(event.target.value);
              setTail(
                Number.isFinite(next) ? Math.min(1000, Math.max(1, next)) : 1,
              );
            }}
          />
          <Button
            variant="outline"
            onClick={() => void refetch()}
            disabled={isFetching}
          >
            <IconRefresh className="size-4" />
            {t("logs.refresh")}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {data?.notices && data.notices.length > 0 && (
          <div className="mb-3 space-y-2">
            {data.notices.map((notice) => (
              <div
                key={notice}
                className="rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-sm text-yellow-800 dark:text-yellow-200"
              >
                {t(`logs.notices.${notice}`, {
                  defaultValue: t("logs.notices.generic"),
                })}
              </div>
            ))}
          </div>
        )}
        {isLoading && (
          <div className="flex h-64 items-center justify-center">
            <Spinner />
          </div>
        )}
        {isError && !isLoading && (
          <div className="flex h-64 items-center justify-center rounded-lg border border-border bg-muted/20 text-sm text-muted-foreground">
            {t("logs.unavailable")}
          </div>
        )}
        {!isLoading && !isError && (
          <LogViewer
            lines={data?.lines ?? []}
            emptyMessage={t("logs.empty")}
            className="h-[520px]"
          />
        )}
      </CardContent>
    </Card>
  );
}
