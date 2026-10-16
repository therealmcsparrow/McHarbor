// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const gitopsKeys = {
  pipelines: ['gitops-pipelines'] as const,
  promotionsAll: ['gitops-promotions'] as const,
  promotions: (pipelineId: string | undefined) => ['gitops-promotions', pipelineId] as const,
  approvals: ['gitops-approvals'] as const,
  previews: ['gitops-previews'] as const,
};
