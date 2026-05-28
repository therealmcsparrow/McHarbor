// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from "react-i18next";
import {
  IconPlugConnected,
  IconPlugX,
  IconShieldLock,
} from "@tabler/icons-react";
import { Badge } from "@resources/components/ui/Badge";
import { Button } from "@resources/components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@resources/components/ui/Card";
import { useSystemTerminal } from "../hooks/useSystemTerminal";

export function SystemTerminalTab() {
  const { t } = useTranslation("system");
  const { termRef, connected, connect, disconnect } = useSystemTerminal();

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2 text-base">
              <IconShieldLock className="size-5 text-muted-foreground" />
              {t("terminal.title")}
            </CardTitle>
            <CardDescription>{t("terminal.description")}</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant={connected ? "success" : "secondary"}>
              {connected ? t("terminal.connected") : t("terminal.disconnected")}
            </Badge>
            {connected ? (
              <Button variant="outline" size="sm" onClick={disconnect}>
                <IconPlugX className="size-4" />
                {t("terminal.disconnect")}
              </Button>
            ) : (
              <Button size="sm" onClick={connect}>
                <IconPlugConnected className="size-4" />
                {t("terminal.connect")}
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          <div className="mb-3 rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-sm text-yellow-800 dark:text-yellow-200">
            {t("terminal.warning")}
          </div>
          <div className="min-h-[520px] overflow-hidden rounded-lg border border-border bg-card">
            <div ref={termRef} className="h-[520px] w-full p-2" />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
