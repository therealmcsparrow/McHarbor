// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { useParams, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import {
  IconInfoCircle,
  IconBox,
  IconLayersSubtract,
  IconFileText,
  IconCode,
  IconVariable,
  IconWebhook,
} from "@tabler/icons-react";
import { ConfirmDialog } from "@resources/components/ui/ConfirmDialog";
import { Button } from "@resources/components/ui/Button";
import { Spinner } from "@resources/components/ui/Spinner";
import { useHeaderSlot } from "@resources/stores/headerSlot";
import { cn } from "@resources/utils/cn";
import {
  useStack,
  useStackAction,
  useDeleteStack,
  useUpdateStack,
} from "../hooks/useStacks";
import { TakeOverDialog } from "../components/TakeOverDialog";
import { OverviewTab } from "../components/tabs/OverviewTab";
import { ServicesTab } from "../components/tabs/ServicesTab";
import { LogsTab } from "../components/tabs/LogsTab";
import { ComposeTab } from "../components/tabs/ComposeTab";
import { EnvironmentTab } from "../components/tabs/EnvironmentTab";
import { LayersTab } from "../components/tabs/LayersTab";
import { WebhooksTab } from "../components/tabs/WebhooksTab";
import { StackDetailHeader } from "./StackDetailHeader";

const DETAIL_TAB_IDS = [
  "overview",
  "services",
  "layers",
  "logs",
  "compose",
  "environment",
  "webhooks",
] as const;
type DetailTabId = (typeof DETAIL_TAB_IDS)[number];

const DETAIL_TAB_ICONS: Record<DetailTabId, typeof IconInfoCircle> = {
  overview: IconInfoCircle,
  services: IconBox,
  layers: IconLayersSubtract,
  logs: IconFileText,
  compose: IconCode,
  environment: IconVariable,
  webhooks: IconWebhook,
};

export default function StackDetailPage() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation("stacks");
  const { data: stack, isLoading } = useStack(name ?? "");
  const action = useStackAction();
  const deleteStack = useDeleteStack();
  const updateStack = useUpdateStack();
  const [activeTab, setActiveTab] = useState<DetailTabId>("overview");
  const [removeOpen, setRemoveOpen] = useState(false);
  const [takeOverOpen, setTakeOverOpen] = useState(false);
  const setHeaderActive = useHeaderSlot((s) => s.setActive);

  // Edit mode state
  const [editing, setEditing] = useState(false);
  const [editDescription, setEditDescription] = useState("");

  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  // Reset edit state when stack changes
  useEffect(() => {
    if (stack) {
      setEditDescription(stack.description ?? "");
    }
  }, [stack]);

  const handleStartEdit = useCallback(() => {
    if (!stack) return;
    setEditDescription(stack.description ?? "");
    setEditing(true);
  }, [stack]);

  const handleCancelEdit = useCallback(() => {
    setEditing(false);
    if (stack) {
      setEditDescription(stack.description ?? "");
    }
  }, [stack]);

  const handleSave = useCallback(() => {
    if (!stack) return;
    updateStack.mutate(
      {
        name: stack.name,
        description: editDescription || undefined,
      },
      {
        onSuccess: () => setEditing(false),
      },
    );
  }, [stack, editDescription, updateStack]);

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!stack) {
    return (
      <div className="py-12 text-center text-muted-foreground">
        {t("detail.stackNotFound")}
      </div>
    );
  }

  const isRunning = stack.status === "running" || stack.status === "partial";
  const isManaged = stack.type === "managed";
  const headerSlot = document.getElementById("header-slot");

  const visibleTabs = isManaged
    ? DETAIL_TAB_IDS
    : DETAIL_TAB_IDS.filter((id) => id !== "environment");

  return (
    <div className="flex h-full flex-col gap-0">
      {headerSlot &&
        createPortal(
          <StackDetailHeader
            stackName={stack.name}
            status={stack.status}
            isManaged={isManaged}
            isRunning={isRunning}
            editing={editing}
            onAction={(a) => action.mutate({ name: stack.name, action: a })}
            onRemove={() => setRemoveOpen(true)}
            onTakeOver={() => setTakeOverOpen(true)}
            onEdit={handleStartEdit}
            onSave={handleSave}
            onCancelEdit={handleCancelEdit}
            saving={updateStack.isPending}
          />,
          headerSlot,
        )}

      <div className="flex h-full flex-col overflow-hidden">
        <div className="border-b border-border bg-card px-5 py-3">
          <nav className="flex w-fit bg-muted rounded-lg p-1 gap-x-1">
            {visibleTabs.map((tabId) => {
              const Icon = DETAIL_TAB_ICONS[tabId];
              return (
                <Button
                  key={tabId}
                  type="button"
                  variant="ghost"
                  onClick={() => setActiveTab(tabId)}
                  className={cn(
                    "h-auto py-2 px-3.5 inline-flex items-center gap-x-1.5 text-sm font-medium rounded-lg transition-colors",
                    activeTab === tabId
                      ? "bg-card text-foreground shadow-sm"
                      : "bg-transparent text-muted-foreground hover:text-primary",
                  )}
                >
                  <Icon className="size-4" />
                  {t(`detail.${tabId}`)}
                </Button>
              );
            })}
          </nav>
        </div>

        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-5">
          {activeTab === "overview" && (
            <OverviewTab
              stack={stack}
              editing={editing}
              description={editDescription}
              onDescriptionChange={setEditDescription}
            />
          )}
          {activeTab === "services" && <ServicesTab stackName={stack.name} />}
          {activeTab === "layers" && (
            <LayersTab stackName={stack.name} services={stack.services} />
          )}
          <div
            className={
              activeTab !== "logs" ? "hidden" : "flex min-h-0 flex-1 flex-col"
            }
          >
            <LogsTab stackName={stack.name} />
          </div>
          <div
            className={
              activeTab !== "compose"
                ? "hidden"
                : "flex min-h-0 flex-1 flex-col"
            }
          >
            <ComposeTab
              stackName={stack.name}
              isManaged={isManaged}
              editing={editing}
            />
          </div>
          {activeTab === "environment" && isManaged && (
            <EnvironmentTab stackName={stack.name} editing={editing} />
          )}
          {activeTab === "webhooks" && isManaged && (
            <WebhooksTab stackName={stack.name} />
          )}
        </div>
      </div>

      <TakeOverDialog
        open={takeOverOpen}
        onOpenChange={setTakeOverOpen}
        stackName={stack.name}
      />

      <ConfirmDialog
        open={removeOpen}
        onOpenChange={(open) => !open && setRemoveOpen(false)}
        title={t("confirm.removeTitle")}
        description={t("confirm.removeDescription", { name: stack.name })}
        confirmLabel={t("confirm.removeLabel")}
        onConfirm={() => {
          deleteStack.mutate(stack.name, {
            onSuccess: () => navigate("/stacks"),
          });
          setRemoveOpen(false);
        }}
        loading={deleteStack.isPending}
      />
    </div>
  );
}
