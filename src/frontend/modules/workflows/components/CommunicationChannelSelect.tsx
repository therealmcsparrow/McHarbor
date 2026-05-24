// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@resources/components/ui/Button";
import { cn } from "@resources/utils/cn";
import { api } from "@core/api/client";

type CommunicationChannel = {
  id: string;
  name: string;
  channelType: string;
  isDefault: boolean;
  enabled: boolean;
};

interface CommunicationChannelSelectProps {
  value: string;
  onChange: (value: string) => void;
  channelType?: string;
}

export function CommunicationChannelSelect({
  value,
  onChange,
  channelType,
}: CommunicationChannelSelectProps) {
  const { t } = useTranslation("common");
  const [open, setOpen] = useState(false);

  const { data: channels } = useQuery({
    queryKey: ["communication-channels-select", channelType ?? "all"],
    queryFn: () =>
      api
        .get<CommunicationChannel[]>("/communication-channels")
        .then((r) =>
          (r.data ?? []).filter(
            (channel) =>
              channel.enabled &&
              (!channelType || channel.channelType === channelType),
          ),
        ),
    staleTime: 30_000,
  });

  const selected = channels?.find((channel) => channel.id === value);
  const placeholder = t("workflows.selectCommunicationChannel", {
    defaultValue: "Select communication channel...",
  });
  const defaultLabel = t("workflows.defaultTransport", {
    defaultValue: "Default",
  });
  const useDefaultLabel = t("workflows.useDefaultCommunicationChannel", {
    defaultValue: "Use default communication channel",
  });
  const emptyLabel = t("workflows.noCommunicationChannelsFound", {
    defaultValue: "No enabled communication channels found",
  });
  const selectedLabel = value
    ? (selected?.name ?? placeholder)
    : useDefaultLabel;

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
        <span className="truncate">{selectedLabel}</span>
        <IconChevronDown className="size-3 shrink-0 text-muted-foreground" />
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
          {!channels || channels.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">
              {emptyLabel}
            </p>
          ) : (
            channels.map((channel) => (
              <Button
                key={channel.id}
                type="button"
                variant="ghost"
                onClick={() => {
                  onChange(channel.id);
                  setOpen(false);
                }}
                className={cn(
                  "flex w-full flex-col gap-0.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors",
                  channel.id === value && "bg-muted/50",
                )}
              >
                <span className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
                  <span className="truncate">{channel.name}</span>
                  {channel.isDefault && (
                    <span className="rounded-full bg-primary/10 px-1.5 py-0.5 text-[9px] font-medium uppercase tracking-wide text-primary">
                      {defaultLabel}
                    </span>
                  )}
                </span>
                <span className="text-[10px] capitalize text-muted-foreground">
                  {channel.channelType}
                </span>
              </Button>
            ))
          )}
        </div>
      )}
    </div>
  );
}
