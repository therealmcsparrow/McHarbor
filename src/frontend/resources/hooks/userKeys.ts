// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const userKeys = {
  all: ['users'] as const,
  detail: (userId: string | undefined) => ['users', userId] as const,
  groups: (userId: string) => ['user-groups', userId] as const,
  rolesAll: ['user-roles'] as const,
  roles: (userId: string) => ['user-roles', userId] as const,
  overrides: (userId: string | null | undefined) => ['user-overrides', userId] as const,
};
