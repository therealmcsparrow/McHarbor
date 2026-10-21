// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type Row = Record<string, unknown>;

/**
 * Why a summary came back disconnected. The server sends a code rather than a
 * sentence so the message can be translated client-side. Mirrors the
 * SummaryReason constants in `modules/kubernetes/model.go`.
 */
export type SummaryReason =
  | 'no_environment'
  | 'not_kubernetes'
  | 'unreachable'
  | 'credentials_refused'
  | 'api_unverified';

export type KubernetesSummary = {
  connected: boolean;
  reason?: SummaryReason;
  version: string;
  namespaceScope: string;
  counts: Record<string, number>;
  namespaces: Row[];
  pods: Row[];
  deployments: Row[];
  replicaSets: Row[];
  statefulSets: Row[];
  daemonSets: Row[];
  services: Row[];
  endpoints: Row[];
  networkPolicies: Row[];
  nodes: Row[];
  events: Row[];
  configMaps: Row[];
  secrets: Row[];
  serviceAccounts: Row[];
  resourceQuotas: Row[];
  limitRanges: Row[];
  persistentVolumeClaims: Row[];
  persistentVolumes: Row[];
  storageClasses: Row[];
  ingresses: Row[];
  jobs: Row[];
  cronJobs: Row[];
  horizontalPodAutoscalers: Row[];
  capabilities: Row[];
};
