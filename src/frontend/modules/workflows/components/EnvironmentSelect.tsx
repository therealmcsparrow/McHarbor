// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@resources/components/ui/Button";
import { cn } from "@resources/utils/cn";
import { api } from "@core/api/client";

type EnvSummary = {
  id: string;
  name: string;
  orchestratorType: string;
  connectionType: string;
};

interface EnvironmentSelectProps {
  value: string;
  onChange: (v: string) => void;
}

export function EnvironmentSelect({ value, onChange }: EnvironmentSelectProps) {
  const { t } = useTranslation("common");
  const [open, setOpen] = useState(false);

  const { data: environments } = useQuery({
    queryKey: ["environments-select"],
    queryFn: () =>
      api
        .get<EnvSummary[]>("/environments")
        .then((r) =>
          (r.data ?? []).filter((e) => e.orchestratorType === "docker"),
        ),
    staleTime: 30_000,
  });

  const selected = environments?.find((e) => e.id === value);
  const displayName =
    selected?.name ?? (value || t("workflows.selectEnvironment"));

  return (
    <div className="relative">
      <Button
        type="button"
        variant="ghost"
        aria-label={t("workflows.selectEnvironment")}
        onClick={() => setOpen(!open)}
        className={cn(
          "flex h-8 w-full items-center justify-between rounded-md border border-input bg-card px-2 text-xs ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          !value && "text-muted-foreground",
        )}
      >
        <span className="truncate">{displayName}</span>
        <svg
          className="size-3 shrink-0 text-muted-foreground"
          viewBox="0 0 12 12"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
        >
          <path d="M3 4.5L6 7.5L9 4.5" />
        </svg>
      </Button>
      {open && (
        <div className="absolute left-0 right-0 top-full z-50 mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-popover py-1 shadow-xl">
          {!environments || environments.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">
              {t("workflows.noEnvironmentsFound")}
            </p>
          ) : (
            environments.map((env) => (
              <Button
                key={env.id}
                type="button"
                variant="ghost"
                onClick={() => {
                  onChange(env.id);
                  setOpen(false);
                }}
                className={cn(
                  "flex w-full flex-col gap-0.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors",
                  env.id === value && "bg-muted/50",
                )}
              >
                <span className="text-xs font-semibold text-foreground">
                  {env.name}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {env.connectionType}
                </span>
              </Button>
            ))
          )}
        </div>
      )}
    </div>
  );
}
