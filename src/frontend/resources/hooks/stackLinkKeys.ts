// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const stackLinkKeys = {
  link: (envId: string | null | undefined, containerId: string | undefined) =>
    ['container-stack-link', envId, containerId] as const,
  linkAll: ['container-stack-link'] as const,
};
