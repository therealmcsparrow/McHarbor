// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { formatGroupedRoleScope, groupRoleAssignments } from './role-assignment-utils';

describe('role assignment utilities', () => {
  it('keeps global and environment roles separate and groups stack roles by role/environment', () => {
    const grouped = groupRoleAssignments([
      {
        id: 'global-admin',
        roleId: 'admin',
        roleName: 'Admin',
        environmentId: null,
        environmentName: null,
        stackName: null,
      },
      {
        id: 'stack-b',
        roleId: 'operator',
        roleName: 'Operator',
        environmentId: 'env-1',
        environmentName: 'Local',
        stackName: 'beta',
      },
      {
        id: 'stack-a',
        roleId: 'operator',
        roleName: 'Operator',
        environmentId: 'env-1',
        environmentName: 'Local',
        stackName: 'alpha',
      },
      {
        id: 'stack-a-duplicate',
        roleId: 'operator',
        roleName: 'Operator',
        environmentId: 'env-1',
        environmentName: 'Local',
        stackName: 'alpha',
      },
    ]);

    expect(grouped).toEqual([
      {
        key: 'global-admin',
        roleId: 'admin',
        roleName: 'Admin',
        environmentId: null,
        environmentName: null,
        stackNames: [],
        assignmentIds: ['global-admin'],
      },
      {
        key: 'stack:operator:env-1',
        roleId: 'operator',
        roleName: 'Operator',
        environmentId: 'env-1',
        environmentName: 'Local',
        stackNames: ['alpha', 'beta'],
        assignmentIds: ['stack-b', 'stack-a', 'stack-a-duplicate'],
      },
    ]);
  });

  it('formats grouped role scopes', () => {
    expect(
      formatGroupedRoleScope(
        {
          key: 'global',
          roleId: 'admin',
          roleName: 'Admin',
          environmentId: null,
          environmentName: null,
          stackNames: [],
          assignmentIds: ['global'],
        },
        'Global',
      ),
    ).toBe('Global');

    expect(
      formatGroupedRoleScope(
        {
          key: 'env',
          roleId: 'viewer',
          roleName: 'Viewer',
          environmentId: 'env-1',
          environmentName: 'Local',
          stackNames: [],
          assignmentIds: ['env'],
        },
        'Global',
      ),
    ).toBe('Local');

    expect(
      formatGroupedRoleScope(
        {
          key: 'stack',
          roleId: 'viewer',
          roleName: 'Viewer',
          environmentId: 'env-1',
          environmentName: 'Local',
          stackNames: ['api', 'web'],
          assignmentIds: ['stack'],
        },
        'Global',
      ),
    ).toBe('Local / api, web');
  });
});
