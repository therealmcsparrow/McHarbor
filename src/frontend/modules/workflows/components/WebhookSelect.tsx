// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@resources/components/ui/Button";
import { cn } from "@resources/utils/cn";
import { api, type PaginatedData } from "@core/api/client";

type WebhookItem = {
  id: string;
  name: string;
  url: string;
  isActive: boolean;
};

interface WebhookSelectProps {
  value: string;
  onChange: (value: string) => void;
}

export function WebhookSelect({ value, onChange }: WebhookSelectProps) {
  const { t } = useTranslation("common");
  const [open, setOpen] = useState(false);

  const { data: webhooks } = useQuery({
    queryKey: ["webhooks-select"],
    queryFn: () =>
      api
        .get<PaginatedData<WebhookItem>>("/webhooks", { per_page: "100" })
        .then((r) => (r.data?.items ?? []).filter((webhook) => webhook.isActive)),
    staleTime: 30_000,
  });

  const selected = webhooks?.find((webhook) => webhook.id === value);
  const placeholder = t("workflows.selectWebhook", {
    defaultValue: "Select webhook...",
  });
  const emptyLabel = t("workflows.noWebhooksFound", {
    defaultValue: "No active webhooks found",
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
        <IconChevronDown className="size-3 shrink-0 text-muted-foreground" />
      </Button>
      {open && (
        <div className="absolute left-0 right-0 top-full z-50 mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-popover py-1 shadow-xl">
          {!webhooks || webhooks.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">
              {emptyLabel}
            </p>
          ) : (
            webhooks.map((webhook) => (
              <Button
                key={webhook.id}
                type="button"
                variant="ghost"
                onClick={() => {
                  onChange(webhook.id);
                  setOpen(false);
                }}
                className={cn(
                  "flex w-full flex-col gap-0.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors",
                  webhook.id === value && "bg-muted/50",
                )}
              >
                <span className="truncate text-xs font-semibold text-foreground">
                  {webhook.name}
                </span>
                <span className="truncate text-[10px] text-muted-foreground">
                  {webhook.url}
                </span>
              </Button>
            ))
          )}
        </div>
      )}
    </div>
  );
}
