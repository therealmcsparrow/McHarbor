// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import {
  IconAlertCircle,
  IconAlertTriangle,
  IconCheck,
  IconCircle,
  IconClock,
  IconHelp,
} from "@tabler/icons-react";
import { Badge } from "@resources/components/ui/Badge";
import { Button } from "@resources/components/ui/Button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@resources/components/ui/Dialog";
import { Spinner } from "@resources/components/ui/Spinner";
import { cn } from "@resources/utils/cn";
import type {
  StorageLocationTestResult,
  StorageLocationTestStep,
} from "../hooks/useStorageLocations";

type StorageLocationTestDialogProps = {
  // The result to display. When null the dialog renders an empty
  // loading state (the parent passes the mutation's `data` once
  // it lands). isPending is reflected in a spinner.
  result: StorageLocationTestResult | null | undefined;
  pending: boolean;
  locationName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const STATUS_LABELS: Record<StorageLocationTestStep["status"], string> = {
  pass: "Pass",
  fail: "Fail",
  warn: "Warning",
  skip: "Skipped",
};

const STATUS_ICON: Record<
  StorageLocationTestStep["status"],
  typeof IconCheck
> = {
  pass: IconCheck,
  fail: IconAlertCircle,
  warn: IconAlertTriangle,
  skip: IconHelp,
};

const STATUS_COLOR: Record<StorageLocationTestStep["status"], string> = {
  pass: "text-emerald-500",
  fail: "text-destructive",
  warn: "text-amber-500",
  skip: "text-muted-foreground",
};

const STATUS_BG: Record<StorageLocationTestStep["status"], string> = {
  pass: "bg-emerald-500/10 border-emerald-500/30",
  fail: "bg-destructive/10 border-destructive/30",
  warn: "bg-amber-500/10 border-amber-500/30",
  skip: "bg-muted/30 border-border",
};

export function StorageLocationTestDialog({
  result,
  pending,
  locationName,
  open,
  onOpenChange,
}: StorageLocationTestDialogProps) {
  // The `common` namespace is added so we can resolve i18n keys
  // like `common.close` that live outside the `settings` namespace.
  // Without it, i18next's resolution chain wouldn't find the key
  // and the button would render the raw key text.
  const { t } = useTranslation(["settings", "common"]);

  const overall = result?.overallStatus ?? (pending ? "running" : "skip");
  const overallVariant: "default" | "destructive" | "secondary" =
    overall === "pass"
      ? "default"
      : overall === "fail"
        ? "destructive"
        : "secondary";

  const overallLabel: string =
    overall === "pass"
      ? t("storage.test.resultPass")
      : overall === "fail"
        ? t("storage.test.resultFail")
        : overall === "warn"
          ? t("storage.test.resultWarn")
          : overall === "running"
            ? t("storage.test.running")
            : t("storage.test.resultSkip");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t("storage.test.title", { name: locationName })}
          </DialogTitle>
          <DialogDescription>
            {pending
              ? t("storage.test.running")
              : t("storage.test.description", { name: locationName })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {pending ? (
            <div
              className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4"
              data-testid="storage-test-running"
            >
              <Spinner size="md" />
              <span className="text-sm text-foreground">
                {t("storage.test.running")}
              </span>
            </div>
          ) : (
            <div data-testid="storage-test-result" className="space-y-3">
              <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t("storage.test.overallStatus")}
                  </span>
                  <Badge variant={overallVariant}>{overallLabel}</Badge>
                </div>
                {result && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <IconClock className="size-3.5" />
                    {result.duration}
                  </span>
                )}
              </div>
              {result && result.steps.length > 0 ? (
                <ul className="space-y-2" data-testid="storage-test-steps">
                  {result.steps.map((step, index) => {
                    const StepIcon = STATUS_ICON[step.status];
                    return (
                      <li
                        key={`${step.name}-${index}`}
                        className={cn(
                          "rounded-lg border p-3 text-sm",
                          STATUS_BG[step.status],
                        )}
                        data-testid={`storage-test-step-${step.name}`}
                      >
                        <div className="flex items-start gap-3">
                          <StepIcon
                            className={cn(
                              "mt-0.5 size-4 shrink-0",
                              STATUS_COLOR[step.status],
                            )}
                            aria-hidden="true"
                          />
                          <div className="min-w-0 flex-1 space-y-1">
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-medium text-foreground">
                                {t(`storage.test.steps.${step.name}`, {
                                  defaultValue: step.name,
                                })}
                              </span>
                              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                                <Badge
                                  variant={
                                    step.status === "fail"
                                      ? "destructive"
                                      : step.status === "warn"
                                        ? "secondary"
                                        : "outline"
                                  }
                                >
                                  {t(
                                    `storage.test.status.${step.status}`,
                                    {
                                      defaultValue: STATUS_LABELS[step.status],
                                    },
                                  )}
                                </Badge>
                                {step.latency && (
                                  <span className="font-mono">
                                    {step.latency}
                                  </span>
                                )}
                              </span>
                            </div>
                            {step.detail && (
                              <p className="break-words text-xs text-muted-foreground">
                                {step.detail}
                              </p>
                            )}
                          </div>
                        </div>
                      </li>
                    );
                  })}
                </ul>
              ) : (
                <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
                  <IconCircle className="size-4" aria-hidden="true" />
                  {t("storage.test.noSteps")}
                </div>
              )}
            </div>
          )}
        </DialogBody>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={pending}
          >
            {t("common.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
