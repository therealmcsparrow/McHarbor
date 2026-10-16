// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ConfigFieldType, NodeDefinition } from '@resources/types/workflows';
import { normalizeRequirement, type NodeRequirement } from './requirements';

export type { ConfigFieldType, NodeDefinition };
export type { NodeRequirement } from './requirements';
export { LEGACY_NODE_ALIASES, isLegacyNodeAction } from './legacyAliases';

export const CATEGORY_COLORS: Record<string, { header: string; border: string; text?: string }> = {
  trigger: { header: 'bg-success', border: 'border-success/30' },
  action: { header: 'bg-info', border: 'border-info/30' },
  logic: { header: 'bg-warning', border: 'border-warning/30' },
  // `text` overrides the canvas theme's near-white header ink. A light
  // background needs dark text, and utility is the only light one.
  utility: {
    header: 'bg-node-utility',
    border: 'border-node-utility/30',
    text: 'text-node-utility-foreground',
  },
  integration: { header: 'bg-chart-3', border: 'border-chart-3/30' },
};

export const CATEGORY_TAG_COLORS: Record<string, string> = {
  trigger: 'bg-success/20 text-success',
  action: 'bg-info/20 text-info',
  logic: 'bg-warning/20 text-warning',
  utility: 'bg-node-utility/20 text-node-utility',
  integration: 'bg-chart-3/20 text-chart-3',
};

// Selection glow per category. Emitted as utility classes rather than as
// an rgb() triple fed to an inline `style`, so the glow resolves through
// the theme tokens and the node component keeps its markup free of
// inline styles.
export const CATEGORY_GLOW_CLASS: Record<string, string> = {
  trigger: 'shadow-lg shadow-success/40 ring-2 ring-success/40',
  action: 'shadow-lg shadow-info/40 ring-2 ring-info/40',
  logic: 'shadow-lg shadow-warning/40 ring-2 ring-warning/40',
  utility: 'shadow-lg shadow-node-utility/40 ring-2 ring-node-utility/40',
  integration: 'shadow-lg shadow-chart-3/40 ring-2 ring-chart-3/40',
};

export const FALLBACK_GLOW_CLASS = 'shadow-lg shadow-primary/40 ring-2 ring-primary/40';

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

export type NodeDocumentation = {
  readme?: string;
  backendAction?: string;
};

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

function extractNodeKey(path: string): string | null {
  const match = path.match(/(?:^|[\\/])nodes[\\/]([^\\/]+)[\\/]/);
  return match?.[1] ?? null;
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

export function getAllNodeDefinitionMap(customNodes: NodeDefinition[]): Record<string, NodeDefinition> {
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

export function isNodeDefinitionAvailable(definition: NodeDefinition, capabilities: Set<string>): boolean {
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
