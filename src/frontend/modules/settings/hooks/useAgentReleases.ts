// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { AGENT_INVENTORY_KEY } from '@resources/hooks/useAgentInventory';

export type AgentReleaseAsset = {
  name: string;
  kind: 'binary' | 'image';
  os: string;
  arch: string;
  sha256: string;
  size: number;
};

export type AgentReleaseStatus = 'pulling' | 'ready' | 'failed';

export type AgentRelease = {
  id: string;
  version: string;
  build: string;
  sourceKind: string;
  sourceUrl: string;
  status: AgentReleaseStatus;
  signatureStatus: '' | 'verified' | 'refused';
  signatureKey: string;
  assets: AgentReleaseAsset[];
  sizeBytes: number;
  imageRef: string;
  imageError: string;
  lastError: string;
  pulledAt: string;
  pulledBy: string;
  isTarget: boolean;
  inUse: boolean;
  /** A build chosen over a newer one of its version; checks leave it in place. */
  pinned?: boolean;
};

export type AgentReleaseSourceKind = 'github' | 'mirror';

export type AgentReleaseSource = { kind: AgentReleaseSourceKind; url: string };

export type AgentReleaseMode = 'notify' | 'download' | 'rollout';

export type AgentReleaseSettings = {
  enabled: boolean;
  mode: AgentReleaseMode;
  checkCron: string;
  keep: number;
  sources: AgentReleaseSource[];
  imageRepository: string;
  imagePlatforms: string[];
};

export type AgentAvailableVersion = {
  version: string;
  build?: string;
};

export type AgentReleaseState = {
  latestVersion?: string;
  latestBuild?: string;
  latestSource?: string;
  lastCheckedAt?: string;
  lastError?: string;
  nextCheckAt?: string;
  available?: AgentAvailableVersion[];
};

export type AgentReleaseOverview = {
  releases: AgentRelease[];
  target: string;
  targetBuild?: string;
  state: AgentReleaseState;
  settings: AgentReleaseSettings;
};

export const AGENT_RELEASES_KEY = ['updates', 'agents', 'releases'] as const;

const BASE = '/updates/agents/releases';

/** One stored build of a version; several builds of one version are kept side by side. */
export type AgentReleaseBuild = { version: string; build?: string };

export function releaseLabel({ version, build }: AgentReleaseBuild): string {
  return build ? `${version} (${build})` : version;
}

export function useAgentReleases() {
  const query = useQuery({
    queryKey: AGENT_RELEASES_KEY,
    queryFn: () => api.get<AgentReleaseOverview>(BASE).then(assertSuccess),
    refetchInterval: (current) => (current.state.data?.releases.some((r) => r.status === 'pulling') ? 2000 : 30000),
    refetchIntervalInBackground: false,
  });
  useRecheckAgentsOnTargetChange(query.data);
  return query;
}

// A download moves the target on the server once it verifies, with no mutation
// here to hang an invalidation on, so the agents table re-runs its up-to-date
// comparison whenever the polled target changes.
function useRecheckAgentsOnTargetChange(data: AgentReleaseOverview | undefined) {
  const queryClient = useQueryClient();
  const seen = useRef<string | null>(null);
  const target = data ? `${data.target}@${data.targetBuild ?? ''}` : null;
  useEffect(() => {
    if (target === null) return;
    if (seen.current !== null && seen.current !== target) {
      void queryClient.invalidateQueries({ queryKey: AGENT_INVENTORY_KEY });
    }
    seen.current = target;
  }, [target, queryClient]);
}

function useReleaseMutation<TVariables>(request: (variables: TVariables) => Promise<unknown>, successKey: string) {
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: request,
    meta: { success: t(successKey) },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: AGENT_RELEASES_KEY });
      void queryClient.invalidateQueries({ queryKey: AGENT_INVENTORY_KEY });
    },
  });
}

export function useCheckAgentReleases() {
  return useReleaseMutation(
    () => api.post<AgentReleaseState>(`${BASE}/check`, {}).then(assertSuccess),
    'agentReleases.toast.checked',
  );
}

/** Pulls one build of a version; without a build the source's newest is fetched. */
export type AgentReleasePull = { version: string; build?: string };

export function usePullAgentRelease() {
  return useReleaseMutation(
    ({ version, build }: AgentReleasePull) =>
      api
        .post<unknown>(
          `${BASE}/${encodeURIComponent(version)}/pull${build ? `?build=${encodeURIComponent(build)}` : ''}`,
          {},
        )
        .then(assertSuccess),
    'agentReleases.toast.pullStarted',
  );
}

export function useDeleteAgentRelease() {
  return useReleaseMutation(
    ({ version, build }: AgentReleaseBuild) =>
      api
        .del<unknown>(`${BASE}/${encodeURIComponent(version)}${build ? `?build=${encodeURIComponent(build)}` : ''}`)
        .then(assertSuccess),
    'agentReleases.toast.deleted',
  );
}

export function useSetAgentTarget() {
  return useReleaseMutation(
    ({ version, build }: AgentReleaseBuild) =>
      api.put<unknown>(`${BASE}/target`, build ? { version, build } : { version }).then(assertSuccess),
    'agentReleases.toast.targetSet',
  );
}

export function useSaveAgentReleaseSettings() {
  return useReleaseMutation(
    (settings: AgentReleaseSettings) => api.put<AgentReleaseSettings>(`${BASE}/settings`, settings).then(assertSuccess),
    'agentReleases.toast.settingsSaved',
  );
}

export type AgentReleaseSourceRow = AgentReleaseSource & { id: string };

let sourceRowSeq = 0;

export function newSourceRow(source: AgentReleaseSource): AgentReleaseSourceRow {
  sourceRowSeq += 1;
  return { ...source, id: `source-${sourceRowSeq}` };
}
