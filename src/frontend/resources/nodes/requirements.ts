// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ConfigFieldType, NodeDefinition } from './registry';

export type NodeRequirement = {
  capabilities: string[];
  mode: 'all' | 'any';
  kind: 'email-server' | 'communication-channel' | 'environment' | 'capability';
};

const FIELD_TYPE_REQUIREMENTS: Partial<Record<ConfigFieldType, NodeRequirement>> = {
  'environment-select': {
    capabilities: ['environment:any'],
    mode: 'all',
    kind: 'environment',
  },
  'container-select': {
    capabilities: ['environment:any'],
    mode: 'all',
    kind: 'environment',
  },
  'email-server-select': {
    capabilities: ['email'],
    mode: 'all',
    kind: 'email-server',
  },
  'communication-channel-select': {
    capabilities: ['channel:any'],
    mode: 'all',
    kind: 'communication-channel',
  },
};
const DOCKER_ENVIRONMENT_NODE_PREFIXES = [
  'compose-',
  'container-',
  'docker-',
  'host-',
  'image-',
  'network-',
  'stack-',
  'volume-',
];
const DOCKER_ENVIRONMENT_NODE_KEYS = new Set([
  'metric-trigger',
  'registry-search',
  'inspect',
  'list',
  'environment-info',
]);

function cloneRequirement(requirement: NodeRequirement): NodeRequirement {
  return {
    ...requirement,
    capabilities: [...requirement.capabilities],
  };
}

export function normalizeRequirement(definition: NodeDefinition): NodeRequirement | undefined {
  if (typeof definition.requires === 'string' && definition.requires.trim() !== '') {
    return {
      capabilities: [definition.requires],
      mode: 'all',
      kind: 'capability',
    };
  }

  if (Array.isArray(definition.requires) && definition.requires.length > 0) {
    const capabilities = definition.requires.filter((value) => typeof value === 'string' && value.trim() !== '');
    if (capabilities.length > 0) {
      return {
        capabilities,
        mode: 'all',
        kind: 'capability',
      };
    }
  }

  const dockerScopedRequirement = inferDockerScopedRequirement(definition);
  if (dockerScopedRequirement) {
    return dockerScopedRequirement;
  }

  const inferred = definition.configSchema
    .filter((field) => !field.showWhen)
    .map((field) => FIELD_TYPE_REQUIREMENTS[field.type])
    .filter((requirement): requirement is NodeRequirement => Boolean(requirement));
  if (inferred.length === 0) {
    return undefined;
  }

  if (inferred.length === 1) {
    const [requirement] = inferred;
    if (requirement) {
      return cloneRequirement(requirement);
    }
  }

  const capabilities = Array.from(new Set(inferred.flatMap((requirement) => requirement.capabilities)));

  return {
    capabilities,
    mode: 'all',
    kind: 'capability',
  };
}

function inferDockerScopedRequirement(definition: NodeDefinition): NodeRequirement | undefined {
  const usesEnvironmentConfig = definition.configSchema.some(
    (field) => field.type === 'environment-select' || field.type === 'container-select',
  );
  if (!usesEnvironmentConfig) {
    return undefined;
  }

  const isDockerScoped =
    DOCKER_ENVIRONMENT_NODE_KEYS.has(definition.key) ||
    DOCKER_ENVIRONMENT_NODE_PREFIXES.some((prefix) => definition.key.startsWith(prefix));
  if (!isDockerScoped) {
    return undefined;
  }

  return {
    capabilities: ['environment:docker'],
    mode: 'all',
    kind: 'environment',
  };
}
