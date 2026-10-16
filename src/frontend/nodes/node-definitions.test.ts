// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import type { NodeDefinition } from '@modules/workflows/types';

const modules = import.meta.glob<Record<string, NodeDefinition>>('@nodes/*/index.ts', { eager: true });

describe('workflow node definitions', () => {
  it('exports a valid node definition from every node folder', () => {
    const definitions = Object.entries(modules).map(([path, module]) => {
      const definition = Object.values(module).find((value) => {
        return value && typeof value === 'object' && 'key' in value && 'configSchema' in value;
      });
      const folder = path.match(/(?:^|[\\/])nodes[\\/]([^\\/]+)[\\/]index\.ts$/)?.[1];
      return { definition, folder, path };
    });

    expect(definitions.length).toBeGreaterThan(80);

    for (const { definition, folder, path } of definitions) {
      expect(definition, `${path} should export a NodeDefinition`).toBeDefined();
      expect(definition?.key).toBe(folder);
      expect(definition?.label).toEqual(expect.any(String));
      expect(definition?.category).toEqual(expect.any(String));
      expect(definition?.description).toEqual(expect.any(String));
      expect(definition?.icon).toEqual(expect.any(String));
      expect(Array.isArray(definition?.configSchema)).toBe(true);
      expect(Array.isArray(definition?.inputPorts)).toBe(true);
      expect(Array.isArray(definition?.outputPorts)).toBe(true);
    }
  });
});
