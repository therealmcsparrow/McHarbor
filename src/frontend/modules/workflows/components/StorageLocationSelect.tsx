// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { IconChevronDown } from "@tabler/icons-react";
import { api } from "@core/api/client";
import { Button } from "@resources/components/ui/Button";
import { cn } from "@resources/utils/cn";

type StorageLocationItem = {
  id: string;
  name: string;
  locationType: string;
  enabled: boolean;
  host?: string;
  basePath?: string;
  bucket?: string;
};

interface StorageLocationSelectProps {
  value: string;
  onChange: (value: string) => void;
}

export function StorageLocationSelect({
  value,
  onChange,
}: StorageLocationSelectProps) {
  const { t } = useTranslation("common");
  const [open, setOpen] = useState(false);

  const { data: locations } = useQuery({
    queryKey: ["workflow-storage-locations-select"],
    queryFn: () =>
      api
        .get<StorageLocationItem[]>("/storage-locations")
        .then((response) => response.data ?? []),
    staleTime: 30_000,
  });

  const enabledLocations = (locations ?? []).filter((location) => location.enabled);
  const selected = enabledLocations.find((location) => location.id === value);
  const placeholder = t("workflows.selectStorageLocation", {
    defaultValue: "Select storage location...",
  });
  const localLabel = t("workflows.useLocalWorkflowFiles", {
    defaultValue: "Use local workflow files",
  });
  const emptyLabel = t("workflows.noStorageLocationsFound", {
    defaultValue: "No enabled storage locations found",
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
        <span className="truncate">{value ? selected?.name ?? placeholder : localLabel}</span>
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
              "flex w-full items-center px-3 py-1.5 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/50",
              !value && "bg-muted/50 text-foreground",
            )}
          >
            {localLabel}
          </Button>
          {enabledLocations.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">{emptyLabel}</p>
          ) : (
            enabledLocations.map((location) => (
              <Button
                key={location.id}
                type="button"
                variant="ghost"
                onClick={() => {
                  onChange(location.id);
                  setOpen(false);
                }}
                className={cn(
                  "flex w-full flex-col gap-0.5 px-3 py-1.5 text-left transition-colors hover:bg-muted/50",
                  location.id === value && "bg-muted/50",
                )}
              >
                <span className="truncate text-xs font-semibold text-foreground">
                  {location.name}
                </span>
                <span className="truncate text-[10px] text-muted-foreground">
                  {storageLocationDetail(location)}
                </span>
              </Button>
            ))
          )}
        </div>
      )}
    </div>
  );
}

function storageLocationDetail(location: StorageLocationItem): string {
  const target =
    location.basePath || location.bucket || location.host || location.locationType;
  return `${location.locationType}${target ? ` - ${target}` : ""}`;
}
