// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import {
  IconActivity,
  IconCpu,
  IconFileText,
  IconInfoCircle,
  IconPackage,
  IconRefresh,
  IconServer,
  IconTerminal2,
} from "@tabler/icons-react";
import type { TablerIcon } from "@tabler/icons-react";
import { PageHeader } from "@resources/layout/PageHeader";
import { Spinner } from "@resources/components/ui/Spinner";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@resources/components/ui/Tabs";
import { useContainersBulkStats } from "@resources/hooks/useContainersBulkStats";
import { useHostMetrics } from "@resources/hooks/useHostMetrics";
import { useSystemInfo } from "../hooks/useSystemInfo";
import { SystemDependenciesTab } from "../components/SystemDependenciesTab";
import { SystemOverviewTab } from "../components/SystemOverviewTab";
import { SystemLogsTab } from "../components/SystemLogsTab";
import { SystemOSUpdatesTab } from "../components/SystemOSUpdatesTab";
import { SystemProcessesTab } from "../components/SystemProcessesTab";
import { SystemRuntimeTab } from "../components/SystemRuntimeTab";
import { SystemServicesTab } from "../components/SystemServicesTab";
import { SystemTerminalTab } from "../components/SystemTerminalTab";

const TAB_IDS = [
  "overview",
  "runtime",
  "services",
  "processes",
  "terminal",
  "logs",
  "updates",
  "dependencies",
] as const;
type TabId = (typeof TAB_IDS)[number];

const TAB_ICONS: Record<TabId, TablerIcon> = {
  overview: IconInfoCircle,
  runtime: IconCpu,
  services: IconServer,
  processes: IconActivity,
  terminal: IconTerminal2,
  logs: IconFileText,
  updates: IconRefresh,
  dependencies: IconPackage,
};

export default function SystemPage() {
  const { t } = useTranslation("system");
  const { data: info, isLoading, isError } = useSystemInfo();
  const { data: hostMetrics } = useHostMetrics();
  const { data: containerMetricsMap, isLoading: isProcessesLoading } =
    useContainersBulkStats();
  const containerMetrics = Array.from(containerMetricsMap?.values() ?? []);

  return (
    <div className="space-y-6">
      <PageHeader title={t("title")} description={t("description")} />

      {isLoading && (
        <div className="flex h-64 items-center justify-center">
          <Spinner size="lg" />
        </div>
      )}

      {(isError || !info) && !isLoading && (
        <div className="flex h-64 items-center justify-center rounded-lg border border-border bg-muted/20 text-sm text-muted-foreground">
          {t("unavailable")}
        </div>
      )}

      {info && (
        <Tabs defaultValue="overview">
          <TabsList className="w-full justify-start overflow-x-auto sm:w-fit">
            {TAB_IDS.map((tabId) => {
              const Icon = TAB_ICONS[tabId];
              return (
                <TabsTrigger key={tabId} value={tabId}>
                  <Icon className="size-4" />
                  {t(`tabs.${tabId}`)}
                </TabsTrigger>
              );
            })}
          </TabsList>

          <TabsContent value="overview">
            <SystemOverviewTab
              info={info}
              hostMetrics={hostMetrics}
              containerMetrics={containerMetrics}
            />
          </TabsContent>
          <TabsContent value="runtime">
            <SystemRuntimeTab info={info} />
          </TabsContent>
          <TabsContent value="services">
            <SystemServicesTab
              info={info}
              hostMetrics={hostMetrics}
              containerMetrics={containerMetrics}
            />
          </TabsContent>
          <TabsContent value="processes">
            <SystemProcessesTab
              processes={containerMetrics}
              isLoading={isProcessesLoading}
            />
          </TabsContent>
          <TabsContent value="terminal">
            <SystemTerminalTab />
          </TabsContent>
          <TabsContent value="logs">
            <SystemLogsTab />
          </TabsContent>
          <TabsContent value="updates">
            <SystemOSUpdatesTab />
          </TabsContent>
          <TabsContent value="dependencies">
            <SystemDependenciesTab info={info} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}
