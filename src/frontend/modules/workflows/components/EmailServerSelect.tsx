// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@resources/components/ui/Button";
import { cn } from "@resources/utils/cn";
import { api } from "@core/api/client";

type EmailServer = {
  id: string;
  name: string;
  serverType: string;
  fromAddress: string;
  isDefault: boolean;
  enabled: boolean;
};

interface EmailServerSelectProps {
  value: string;
  onChange: (value: string) => void;
}

export function EmailServerSelect({ value, onChange }: EmailServerSelectProps) {
  const { t } = useTranslation("common");
  const [open, setOpen] = useState(false);

  const { data: servers } = useQuery({
    queryKey: ["email-servers-select"],
    queryFn: () =>
      api
        .get<EmailServer[]>("/email-servers")
        .then((r) => (r.data ?? []).filter((server) => server.enabled)),
    staleTime: 30_000,
  });

  const selected = servers?.find((server) => server.id === value);
  const placeholder = t("workflows.selectEmailServer", {
    defaultValue: "Select email server...",
  });
  const defaultLabel = t("workflows.defaultTransport", {
    defaultValue: "Default",
  });
  const useDefaultLabel = t("workflows.useDefaultEmailServer", {
    defaultValue: "Use default email server",
  });
  const emptyLabel = t("workflows.noEmailServersFound", {
    defaultValue: "No enabled email servers found",
  });

  return (
    <div className="relative">
      <Button
        type="button"
        variant="ghost"
        aria-label={placeholder}
        onClick={() => setOpen((prev) => !prev)}
        className={cn(
          "flex h-8 w-full items-center justify-between rounded-md border border-input bg-card px-2 text-xs ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          !value && "text-muted-foreground",
        )}
      >
        <span className="truncate">{selected?.name ?? placeholder}</span>
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
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              onChange("");
              setOpen(false);
            }}
            className={cn(
              "flex w-full items-center px-3 py-1.5 text-left text-xs text-muted-foreground hover:bg-muted/50 transition-colors",
              !value && "bg-muted/50 text-foreground",
            )}
          >
            {useDefaultLabel}
          </Button>
          {!servers || servers.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">
              {emptyLabel}
            </p>
          ) : (
            servers.map((server) => (
              <Button
                key={server.id}
                type="button"
                variant="ghost"
                onClick={() => {
                  onChange(server.id);
                  setOpen(false);
                }}
                className={cn(
                  "flex w-full flex-col gap-0.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors",
                  server.id === value && "bg-muted/50",
                )}
              >
                <span className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
                  <span className="truncate">{server.name}</span>
                  {server.isDefault && (
                    <span className="rounded-full bg-primary/10 px-1.5 py-0.5 text-[9px] font-medium uppercase tracking-wide text-primary">
                      {defaultLabel}
                    </span>
                  )}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {server.serverType}
                  {server.fromAddress ? ` - ${server.fromAddress}` : ""}
                </span>
              </Button>
            ))
          )}
        </div>
      )}
    </div>
  );
}
