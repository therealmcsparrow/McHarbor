// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type StackSvc = {
  name: string;
  containerId?: string;
  status: string;
  image: string;
};

export type StackInfo = {
  id: string;
  name: string;
  status: string;
  services: StackSvc[];
  description?: string;
  type: 'managed' | 'discovered';
  protected?: boolean;
  targetsSelf: boolean;
  environmentId?: string;
  ownerUserId?: string;
  createdAt?: string;
  updatedAt?: string;
  orchestrator?: string;
  ownerName?: string;
};

export type StackDetail = {
  id: string;
  name: string;
  status: string;
  services: StackSvc[];
  description?: string;
  type: 'managed' | 'discovered';
  secretFileMode: boolean;
  targetsSelf: boolean;
  environmentId?: string;
  createdAt: string;
  updatedAt: string;
};
