// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api } from "@core/api/client";
import { useEnvironmentStore } from "@resources/stores/environment";
import type {
  OSLogResult,
  OSLogSource,
  OSUpdateApplyResult,
  OSUpdateCheckResult,
} from "../types";

export function useOSLogs(source: OSLogSource, tail: number) {
  const { t } = useTranslation("system");
  const envId = useEnvironmentStore((s) => s.currentId);

  return useQuery({
    queryKey: ["system-os-logs", envId, source, tail],
    queryFn: async () => {
      const params: Record<string, string> = { source, tail: String(tail) };
      if (envId) params.env = envId;

      const result = await api.get<OSLogResult>("/system/os-logs", params);
      if (!result.success || !result.data) {
        throw new Error(result.error ?? t("logs.unavailable"));
      }
      return result.data;
    },
    staleTime: 15_000,
  });
}

export function useOSUpdateCheck() {
  const { t } = useTranslation("system");
  const envId = useEnvironmentStore((s) => s.currentId);

  return useQuery({
    queryKey: ["system-os-updates-check", envId],
    queryFn: async () => {
      const params = envId ? { env: envId } : undefined;
      const result = await api.get<OSUpdateCheckResult>(
        "/system/os-updates/check",
        params,
      );
      if (!result.success || !result.data) {
        throw new Error(result.error ?? t("updates.checkFailed"));
      }
      return result.data;
    },
    enabled: false,
    staleTime: 60_000,
  });
}

export function useApplyOSUpdates() {
  const { t } = useTranslation("system");
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const path = envId
        ? `/system/os-updates/apply?${new URLSearchParams({ env: envId }).toString()}`
        : "/system/os-updates/apply";
      const result = await api.post<OSUpdateApplyResult>(path, {
        confirm: true,
      });
      if (!result.success || !result.data) {
        throw new Error(result.error ?? t("updates.applyFailed"));
      }
      return result.data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["system-os-updates-check", envId],
      });
    },
    meta: { success: t("updates.applyStarted") },
  });
}
