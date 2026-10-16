// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { SECRET_SCOPES, type Secret } from '@core/types/secrets';

export type SecretGroupNode = {
  key: string;
  group: string;
  secrets: Secret[];
};

export type SecretKindNode = {
  key: string;
  kind: string;
  count: number;
  groups: SecretGroupNode[];
};

export type SecretScopeNode = {
  key: string;
  scope: string;
  count: number;
  kinds: SecretKindNode[];
};

const SCOPE_ORDER = new Map(SECRET_SCOPES.map((scope, index) => [scope as string, index]));

function byScope(a: string, b: string): number {
  const left = SCOPE_ORDER.get(a) ?? SCOPE_ORDER.size;
  const right = SCOPE_ORDER.get(b) ?? SCOPE_ORDER.size;
  return left === right ? a.localeCompare(b) : left - right;
}

function byGroup(a: string, b: string): number {
  if (a === '') return 1;
  if (b === '') return -1;
  return a.localeCompare(b);
}

export function buildSecretTree(secrets: Secret[]): SecretScopeNode[] {
  const tree = new Map<string, Map<string, Map<string, Secret[]>>>();

  for (const secret of secrets) {
    const group = secret.group?.trim() ?? '';
    const kinds = tree.get(secret.scope) ?? new Map<string, Map<string, Secret[]>>();
    const groups = kinds.get(secret.kind) ?? new Map<string, Secret[]>();
    groups.set(group, [...(groups.get(group) ?? []), secret]);
    kinds.set(secret.kind, groups);
    tree.set(secret.scope, kinds);
  }

  return [...tree.entries()]
    .sort(([a], [b]) => byScope(a, b))
    .map(([scope, kinds]) => {
      const kindNodes = [...kinds.entries()]
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([kind, groups]) => ({
          key: `${scope}/${kind}`,
          kind,
          count: [...groups.values()].reduce((total, rows) => total + rows.length, 0),
          groups: [...groups.entries()]
            .sort(([a], [b]) => byGroup(a, b))
            .map(([group, rows]) => ({
              key: `${scope}/${kind}/${group}`,
              group,
              secrets: [...rows].sort((a, b) => a.name.localeCompare(b.name)),
            })),
        }));

      return {
        key: scope,
        scope,
        count: kindNodes.reduce((total, node) => total + node.count, 0),
        kinds: kindNodes,
      };
    });
}
