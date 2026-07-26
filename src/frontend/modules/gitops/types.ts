// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type TriggerType = 'auto' | 'manual' | 'pr_preview';
export type DeployMode = 'auto' | 'manual' | 'pr_preview';
export type PromotionStatus = 'pending' | 'succeeded' | 'failed' | 'rolled_back' | 'skipped';
export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'expired';
export type TriggerKind = 'auto' | 'manual' | 'pr_preview' | 'webhook';
export type PreviewStatus = 'active' | 'demolished' | 'failed';

export type GitOpsStage = {
  id: string;
  pipelineId: string;
  stageName: string;
  stageIndex: number;
  targetEnvironmentId: string;
  deployMode: DeployMode;
  branch: string;
  composePath: string;
  requiresApproval: boolean;
  createdAt: string;
  updatedAt: string;
};

export type GitOpsPipeline = {
  id: string;
  name: string;
  repoId: string;
  description: string;
  enabled: boolean;
  triggerType: TriggerType;
  stages: GitOpsStage[];
  createdAt: string;
  updatedAt: string;
};

export type GitOpsPromotion = {
  id: string;
  pipelineId: string;
  stageId: string;
  deploymentId: string;
  commitSha: string;
  fromStageId: string;
  status: PromotionStatus;
  triggerKind: TriggerKind;
  prNumber: string;
  triggeredBy: string;
  note: string;
  startedAt: string;
  finishedAt: string;
  createdAt: string;
  updatedAt: string;
};

export type GitOpsApproval = {
  id: string;
  pipelineId: string;
  stageId: string;
  commitSha: string;
  requestedBy: string;
  requestedAt: string;
  resolvedBy: string;
  resolvedAt: string;
  status: ApprovalStatus;
  note: string;
  createdAt: string;
  updatedAt: string;
};

export type GitOpsPRPreview = {
  id: string;
  pipelineId: string;
  prNumber: string;
  prTitle: string;
  sourceBranch: string;
  targetBranch: string;
  commitSha: string;
  previewEnvironmentId: string;
  status: PreviewStatus;
  openedAt: string;
  closedAt: string;
};

export type StageInput = {
  stageName: string;
  stageIndex: number;
  targetEnvironmentId: string;
  deployMode: DeployMode;
  branch: string;
  composePath: string;
  requiresApproval: boolean;
};

export type CreatePipelineInput = {
  name: string;
  repoId: string;
  description: string;
  triggerType: TriggerType;
  stages: StageInput[];
};

export type UpdatePipelineInput = {
  name?: string;
  description?: string;
  enabled?: boolean;
  triggerType?: TriggerType;
  stages?: StageInput[];
};

export type PromoteInput = {
  stageId: string;
  commitSha: string;
  note: string;
  triggeredBy: string;
  prNumber: string;
};

export type ApprovalInput = {
  note: string;
  resolvedBy: string;
};
