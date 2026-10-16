// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { gitopsKeys } from '@modules/gitops/hooks/queryKeys';
import { updatesKeys } from '@modules/settings/hooks/queryKeys';
import { stackLinkKeys } from '@resources/hooks/stackLinkKeys';
import { containerBackupKeys } from '@resources/hooks/containerBackups/queryKeys';
import { userKeys } from '@resources/hooks/userKeys';

describe('query key factories keep the pre-factory key values', () => {
  it('gitops', () => {
    expect(gitopsKeys.pipelines).toEqual(['gitops-pipelines']);
    expect(gitopsKeys.promotionsAll).toEqual(['gitops-promotions']);
    expect(gitopsKeys.promotions('p1')).toEqual(['gitops-promotions', 'p1']);
    expect(gitopsKeys.promotions(undefined)).toEqual(['gitops-promotions', undefined]);
    expect(gitopsKeys.approvals).toEqual(['gitops-approvals']);
    expect(gitopsKeys.previews).toEqual(['gitops-previews']);
  });
  it('updates', () => {
    expect(updatesKeys.check).toEqual(['updates', 'check']);
    expect(updatesKeys.self).toEqual(['updates', 'self']);
    expect(updatesKeys.selfSettings).toEqual(['updates', 'self', 'settings']);
    expect(updatesKeys.policies).toEqual(['updates', 'policies']);
  });
  it('stack links', () => {
    expect(stackLinkKeys.link('e1', 'c1')).toEqual(['container-stack-link', 'e1', 'c1']);
    expect(stackLinkKeys.linkAll).toEqual(['container-stack-link']);
  });
  it('container backups', () => {
    expect(containerBackupKeys.runs('e1', 'c1')).toEqual(['container-backup-runs', 'e1', 'c1']);
    expect(containerBackupKeys.runsAll).toEqual(['container-backup-runs']);
    expect(containerBackupKeys.plansAll).toEqual(['container-backup-plans']);
  });
  it('users', () => {
    expect(userKeys.all).toEqual(['users']);
    expect(userKeys.detail('u1')).toEqual(['users', 'u1']);
    expect(userKeys.groups('u1')).toEqual(['user-groups', 'u1']);
    expect(userKeys.rolesAll).toEqual(['user-roles']);
    expect(userKeys.roles('u1')).toEqual(['user-roles', 'u1']);
    expect(userKeys.overrides('u1')).toEqual(['user-overrides', 'u1']);
  });
});
