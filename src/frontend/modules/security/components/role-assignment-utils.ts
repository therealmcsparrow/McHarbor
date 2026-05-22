// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

type RoleAssignmentScope = {
  id: string;
  roleId: string;
  roleName: string;
  environmentId: string | null;
  environmentName: string | null;
  stackName: string | null;
};

export type GroupedRoleAssignment = {
  key: string;
  roleId: string;
  roleName: string;
  environmentId: string | null;
  environmentName: string | null;
  stackNames: string[];
  assignmentIds: string[];
};

export function groupRoleAssignments<T extends RoleAssignmentScope>(roles: T[]): GroupedRoleAssignment[] {
  const grouped: GroupedRoleAssignment[] = [];
  const stackScoped = new Map<string, GroupedRoleAssignment>();

  for (const role of roles) {
    if (!role.stackName) {
      grouped.push({
        key: role.id,
        roleId: role.roleId,
        roleName: role.roleName,
        environmentId: role.environmentId,
        environmentName: role.environmentName,
        stackNames: [],
        assignmentIds: [role.id],
      });
      continue;
    }

    const key = `${role.roleId}:${role.environmentId ?? ''}`;
    let group = stackScoped.get(key);
    if (!group) {
      group = {
        key: `stack:${key}`,
        roleId: role.roleId,
        roleName: role.roleName,
        environmentId: role.environmentId,
        environmentName: role.environmentName,
        stackNames: [],
        assignmentIds: [],
      };
      stackScoped.set(key, group);
      grouped.push(group);
    }

    group.stackNames.push(role.stackName);
    group.assignmentIds.push(role.id);
  }

  for (const role of grouped) {
    if (role.stackNames.length === 0) {
      continue;
    }

    role.stackNames = Array.from(new Set(role.stackNames)).sort((left, right) => left.localeCompare(right));
  }

  return grouped;
}

export function formatGroupedRoleScope(role: GroupedRoleAssignment, globalLabel: string) {
  if (role.stackNames.length > 0) {
    const stacks = role.stackNames.join(', ');
    return role.environmentName ? `${role.environmentName} / ${stacks}` : stacks;
  }

  return role.environmentName || globalLabel;
}
