// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ConfigFieldType, NodeDefinition } from '@modules/workflows/types';

export const CATEGORY_COLORS: Record<string, { header: string; border: string }> = {
  trigger: { header: 'bg-emerald-600', border: 'border-emerald-500/30' },
  action: { header: 'bg-blue-600', border: 'border-blue-500/30' },
  logic: { header: 'bg-amber-600', border: 'border-amber-500/30' },
  utility: { header: 'bg-slate-600', border: 'border-slate-500/30' },
  integration: { header: 'bg-purple-600', border: 'border-purple-500/30' },
};

export const CATEGORY_TAG_COLORS: Record<string, string> = {
  trigger: 'bg-emerald-500/20 text-emerald-400',
  action: 'bg-blue-500/20 text-blue-400',
  logic: 'bg-amber-500/20 text-amber-400',
  utility: 'bg-slate-500/20 text-slate-400',
  integration: 'bg-purple-500/20 text-purple-400',
};

export const CATEGORY_GLOW_RGB: Record<string, string> = {
  trigger: '52, 211, 153',
  action: '96, 165, 250',
  logic: '251, 191, 36',
  utility: '100, 116, 139',
  integration: '167, 139, 250',
};

const categoryOrder = ['trigger', 'action', 'logic', 'utility', 'integration'];

const nodeModules = import.meta.glob<Record<string, unknown>>('@nodes/*/index.ts', {
  eager: true,
});
const readmeModules = import.meta.glob<string>('@nodes/*/README.md', {
  import: 'default',
  query: '?raw',
});
const backendActionModules = import.meta.glob<string>('@nodes/*/backend-action.md', {
  import: 'default',
  query: '?raw',
});

export type NodeRequirement = {
  capabilities: string[];
  mode: 'all' | 'any';
  kind: 'email-server' | 'communication-channel' | 'environment' | 'capability';
};

export type NodeDocumentation = {
  readme?: string;
  backendAction?: string;
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
const DOCKER_ENVIRONMENT_NODE_KEYS = new Set(['metric-trigger', 'registry-search']);

function isNodeDefinition(value: unknown): value is NodeDefinition {
  return Boolean(
    value &&
      typeof value === 'object' &&
      typeof (value as NodeDefinition).key === 'string' &&
      typeof (value as NodeDefinition).category === 'string' &&
      Array.isArray((value as NodeDefinition).configSchema) &&
      Array.isArray((value as NodeDefinition).inputPorts) &&
      Array.isArray((value as NodeDefinition).outputPorts),
  );
}

function collectNodeDefinitions(value: unknown): NodeDefinition[] {
  if (isNodeDefinition(value)) {
    return [value];
  }
  if (Array.isArray(value)) {
    return value.flatMap((item) => collectNodeDefinitions(item));
  }
  return [];
}

function cloneRequirement(requirement: NodeRequirement): NodeRequirement {
  return {
    ...requirement,
    capabilities: [...requirement.capabilities],
  };
}

function extractNodeKey(path: string): string | null {
  const match = path.match(/(?:^|[\\/])nodes[\\/]([^\\/]+)[\\/]/);
  return match?.[1] ?? null;
}

function normalizeRequirement(definition: NodeDefinition): NodeRequirement | undefined {
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

  const capabilities = Array.from(
    new Set(inferred.flatMap((requirement) => requirement.capabilities)),
  );

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

export const NODE_DEFINITIONS: NodeDefinition[] = Object.values(nodeModules)
  .flatMap((mod) => Object.values(mod).flatMap((value) => collectNodeDefinitions(value)))
  .sort((a, b) => {
    const catDiff = categoryOrder.indexOf(a.category) - categoryOrder.indexOf(b.category);
    if (catDiff !== 0) return catDiff;
    return a.key.localeCompare(b.key);
  });

export const NODE_DEFINITION_MAP: Record<string, NodeDefinition> = Object.fromEntries(
  NODE_DEFINITIONS.map((d) => [d.key, d]),
);
export const NODE_REQUIREMENT_MAP: Record<string, NodeRequirement> = Object.fromEntries(
  NODE_DEFINITIONS.flatMap((definition) => {
    const requirement = normalizeRequirement(definition);
    return requirement ? [[definition.key, requirement]] : [];
  }),
);

const nodeDocumentationCache = new Map<string, Promise<NodeDocumentation | undefined>>();

export function getAllNodeDefinitions(customNodes: NodeDefinition[]): NodeDefinition[] {
  if (customNodes.length === 0) return NODE_DEFINITIONS;
  const builtinKeys = new Set(NODE_DEFINITIONS.map((d) => d.key));
  const unique = customNodes.filter((d) => !builtinKeys.has(d.key));
  const combined = [...NODE_DEFINITIONS, ...unique];
  return combined.sort((a, b) => {
    const catDiff = categoryOrder.indexOf(a.category) - categoryOrder.indexOf(b.category);
    if (catDiff !== 0) return catDiff;
    return a.key.localeCompare(b.key);
  });
}

export function getAllNodeDefinitionMap(
  customNodes: NodeDefinition[],
): Record<string, NodeDefinition> {
  if (customNodes.length === 0) return NODE_DEFINITION_MAP;
  const map = { ...NODE_DEFINITION_MAP };
  for (const d of customNodes) {
    if (!map[d.key]) {
      map[d.key] = d;
    }
  }
  return map;
}

export function getNodeRequirement(definition: NodeDefinition): NodeRequirement | undefined {
  return NODE_REQUIREMENT_MAP[definition.key] ?? normalizeRequirement(definition);
}

export function isNodeDefinitionAvailable(
  definition: NodeDefinition,
  capabilities: Set<string>,
): boolean {
  const requirement = getNodeRequirement(definition);
  if (!requirement) {
    return true;
  }

  if (requirement.mode === 'any') {
    return requirement.capabilities.some((capability) => capabilities.has(capability));
  }

  return requirement.capabilities.every((capability) => capabilities.has(capability));
}

export async function getNodeDocumentation(key: string): Promise<NodeDocumentation | undefined> {
  const cached = nodeDocumentationCache.get(key);
  if (cached) {
    return cached;
  }

  const promise = (async () => {
    const readmeEntry = Object.entries(readmeModules).find(([path]) => extractNodeKey(path) === key);
    const backendEntry = Object.entries(backendActionModules).find(([path]) => extractNodeKey(path) === key);

    const [readme, backendAction] = await Promise.all([
      readmeEntry ? readmeEntry[1]().catch(() => undefined) : Promise.resolve(undefined),
      backendEntry ? backendEntry[1]().catch(() => undefined) : Promise.resolve(undefined),
    ]);

    if (!readme && !backendAction) {
      return undefined;
    }

    return {
      readme,
      backendAction,
    };
  })();

  nodeDocumentationCache.set(key, promise);
  return promise;
}
