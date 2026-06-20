// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import {
  IconInfoCircle,
  IconPackage,
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
import { useSystemInfo } from "../hooks/useSystemInfo";
import { SystemDependenciesTab } from "../components/SystemDependenciesTab";
import { SystemOverviewTab } from "../components/SystemOverviewTab";

const TAB_IDS = ["overview", "dependencies"] as const;
type TabId = (typeof TAB_IDS)[number];

const TAB_ICONS: Record<TabId, TablerIcon> = {
  overview: IconInfoCircle,
  dependencies: IconPackage,
};

export default function SystemPage() {
  const { t } = useTranslation("system");
  const { data: info, isLoading, isError } = useSystemInfo();

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
            <SystemOverviewTab info={info} />
          </TabsContent>
          <TabsContent value="dependencies">
            <SystemDependenciesTab info={info} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}
