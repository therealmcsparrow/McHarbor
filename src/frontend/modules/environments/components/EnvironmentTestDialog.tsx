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
  IconBrandDocker,
  IconServer,
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
  EnvironmentTestResult,
  EnvironmentTestStep,
} from "../hooks/useEnvironments";

type EnvironmentTestDialogProps = {
  // Result to display. When null the dialog renders an empty
  // loading state (the parent passes the mutation's `data` once
  // it lands). isPending is reflected in a spinner.
  result: EnvironmentTestResult | null | undefined;
  pending: boolean;
  envName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const STATUS_ICON: Record<EnvironmentTestStep["status"], typeof IconCheck> = {
  pass: IconCheck,
  fail: IconAlertCircle,
  warn: IconAlertTriangle,
  skip: IconHelp,
};

const STATUS_COLOR: Record<EnvironmentTestStep["status"], string> = {
  pass: "text-emerald-500",
  fail: "text-destructive",
  warn: "text-amber-500",
  skip: "text-muted-foreground",
};

const STATUS_BG: Record<EnvironmentTestStep["status"], string> = {
  pass: "bg-emerald-500/10 border-emerald-500/30",
  fail: "bg-destructive/10 border-destructive/30",
  warn: "bg-amber-500/10 border-amber-500/30",
  skip: "bg-muted/30 border-border",
};

// Map backend step names to a translated, user-facing label.
// Falls back to the raw step name when the i18n key is missing (which
// happens for new backend steps whose translations have not yet been
// added).
const STEP_LABEL_KEYS: Record<string, string> = {
  load: "envs.test.steps.load",
  reset: "envs.test.steps.reset",
  connect: "envs.test.steps.connect",
  ping: "envs.test.steps.ping",
  version: "envs.test.steps.version",
  persist: "envs.test.steps.persist",
};

export function EnvironmentTestDialog({
  result,
  pending,
  envName,
  open,
  onOpenChange,
}: EnvironmentTestDialogProps) {
  const { t } = useTranslation(["environments", "common"]);

  const overall = result?.overall ?? (pending ? "running" : "skip");
  const overallVariant: "default" | "destructive" | "secondary" =
    overall === "pass"
      ? "default"
      : overall === "fail"
        ? "destructive"
        : "secondary";

  const overallLabel: string =
    overall === "pass"
      ? t("envs.test.resultPass")
      : overall === "fail"
        ? t("envs.test.resultFail")
        : overall === "warn"
          ? t("envs.test.resultWarn")
          : overall === "running"
            ? t("envs.test.running")
            : t("envs.test.resultSkip");

  const OrchestratorIcon =
    result?.orchestrator === "kubernetes" ? IconServer : IconBrandDocker;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            <span className="flex items-center gap-2">
              <OrchestratorIcon
                className="size-4 text-muted-foreground"
                aria-hidden="true"
              />
              {t("envs.test.title", { name: envName })}
            </span>
          </DialogTitle>
          <DialogDescription>
            {pending
              ? t("envs.test.running")
              : t("envs.test.description", { name: envName })}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-3">
          {pending ? (
            <div
              className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4"
              data-testid="env-test-running"
            >
              <Spinner size="md" />
              <span className="text-sm text-foreground">
                {t("envs.test.running")}
              </span>
            </div>
          ) : (
            <div data-testid="env-test-result" className="space-y-3">
              <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t("envs.test.overallStatus")}
                  </span>
                  <Badge variant={overallVariant}>{overallLabel}</Badge>
                  {result?.dockerVersion && (
                    <span className="text-xs text-muted-foreground">
                      Docker v{result.dockerVersion}
                    </span>
                  )}
                  {result?.k8sVersion && (
                    <span className="text-xs text-muted-foreground">
                      Kubernetes v{result.k8sVersion}
                    </span>
                  )}
                </div>
                {result && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <IconClock className="size-3.5" />
                    {result.duration}
                  </span>
                )}
              </div>

              {result && result.steps.length > 0 ? (
                <ul
                  className="space-y-2"
                  data-testid="env-test-steps"
                >
                  {result.steps.map((step, index) => {
                    const StepIcon = STATUS_ICON[step.status];
                    const labelKey = STEP_LABEL_KEYS[step.name];
                    const label = labelKey
                      ? t(labelKey, { defaultValue: step.name })
                      : step.name;
                    return (
                      <li
                        key={`${step.name}-${index}`}
                        className={cn(
                          "rounded-lg border p-3 text-sm",
                          STATUS_BG[step.status],
                        )}
                        data-testid={`env-test-step-${step.name}`}
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
                                {label}
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
                                    `envs.test.status.${step.status}`,
                                    { defaultValue: step.status },
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
                  {t("envs.test.noSteps")}
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
            {t("close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}