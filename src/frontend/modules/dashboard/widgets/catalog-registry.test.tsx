// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import {
  BUILTIN_WIDGET_DEFINITIONS,
  BUILTIN_WIDGET_META,
  WIDGET_CATEGORIES,
  getWidgetMeta,
  type WidgetTypeId,
} from './catalog';
import { getWidgetDefinitionMap, WIDGET_REGISTRY } from './registry';
import { useWidgetRegistryStore } from '../stores/widget-registry';

describe('dashboard widget catalog', () => {
  it('keeps metadata IDs, categories, and size constraints consistent', () => {
    const ids = Object.keys(BUILTIN_WIDGET_META) as WidgetTypeId[];
    const categoryIds = new Set(WIDGET_CATEGORIES.map((category) => category.id));

    expect(ids).toHaveLength(37);
    expect(BUILTIN_WIDGET_DEFINITIONS.map((definition) => definition.id).sort()).toEqual(ids.sort());

    for (const definition of BUILTIN_WIDGET_DEFINITIONS) {
      expect(definition.id).toBe(getWidgetMeta(definition.id)?.id);
      expect(categoryIds.has(definition.category)).toBe(true);
      expect(definition.labelKey).toMatch(/^widgets\./);
      expect(definition.descriptionKey).toMatch(/^widgets\./);
      expect(definition.defaultSize.w).toBeGreaterThanOrEqual(definition.minSize.w);
      expect(definition.defaultSize.h).toBeGreaterThanOrEqual(definition.minSize.h);
      expect(definition.icon).toBeTruthy();
    }

    expect(getWidgetMeta('missing-widget')).toBeUndefined();
  });
});

describe('dashboard widget registry', () => {
  beforeEach(() => {
    useWidgetRegistryStore.getState().setDefinitions(Object.values(WIDGET_REGISTRY));
  });

  it('registers every built-in widget with a component', () => {
    expect(Object.keys(WIDGET_REGISTRY).sort()).toEqual(
      BUILTIN_WIDGET_DEFINITIONS.map((definition) => definition.id).sort(),
    );

    for (const definition of Object.values(WIDGET_REGISTRY)) {
      expect(definition.component).toBeTypeOf('object');
      expect(definition.labelKey).toMatch(/^widgets\./);
    }
  });

  it('exposes the current registry map from the Zustand store', () => {
    const map = getWidgetDefinitionMap();

    expect(map.containers?.id).toBe('containers');
    expect(map['stack-status']?.category).toBe(getWidgetMeta('stack-status')?.category);
  });
});
