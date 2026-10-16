// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconServer } from '@tabler/icons-react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WidgetDefinition } from '../widgets/registry';
import { useWidgetRegistryStore } from './widget-registry';

const { addResourceBundle } = vi.hoisted(() => ({ addResourceBundle: vi.fn() }));

vi.mock('@core/i18n/i18n', () => ({
  default: { addResourceBundle },
}));

const widget: WidgetDefinition = {
  id: 'host-info',
  labelKey: 'hostInfo.label',
  category: 'host',
  descriptionKey: 'hostInfo.description',
  icon: IconServer,
  component: () => null,
  defaultSize: { w: 2, h: 2 },
  minSize: { w: 1, h: 1 },
};

describe('useWidgetRegistryStore', () => {
  beforeEach(() => {
    useWidgetRegistryStore.setState({ definitions: [], definitionMap: {} });
    vi.clearAllMocks();
  });

  it('sets definitions and builds a map by ID', () => {
    useWidgetRegistryStore.getState().setDefinitions([widget]);

    expect(useWidgetRegistryStore.getState().definitions).toEqual([widget]);
    expect(useWidgetRegistryStore.getState().definitionMap['host-info']).toBe(widget);
  });

  it('ignores downloaded widgets that already exist and injects translations', async () => {
    useWidgetRegistryStore.getState().setDefinitions([widget]);
    useWidgetRegistryStore.getState().mergeDownloaded([
      {
        key: 'host-info',
        label: 'Host info',
        category: 'system',
        description: 'Host details',
        icon: 'IconServer',
        source: 'store',
        component: 'HostInfoWidget',
        enabled: true,
        defaultSize: { w: 2, h: 2 },
        minSize: { w: 1, h: 1 },
        translations: { en: { title: 'Host' } },
      },
    ]);

    await vi.waitFor(async () => {
      expect(addResourceBundle).toHaveBeenCalledWith(
        'en',
        'dashboard',
        { widgets: { 'host-info': { title: 'Host' } } },
        true,
        false,
      );
    });
    expect(useWidgetRegistryStore.getState().definitions).toEqual([widget]);
  });

  it('registers only server-verified downloaded widgets, and drops one that stops verifying', () => {
    const downloaded = {
      key: 'clock',
      label: 'Clock',
      category: 'plugin',
      description: 'Shows the time',
      icon: 'IconClock',
      source: 'downloaded',
      component: 'bundle.js',
      enabled: true,
      defaultSize: { w: 3, h: 2 },
      minSize: { w: 2, h: 1 },
      version: '2026.09.01',
    };
    useWidgetRegistryStore.getState().setDefinitions([widget]);
    useWidgetRegistryStore.getState().mergeDownloaded([{ ...downloaded, verified: true }]);
    expect(Object.keys(useWidgetRegistryStore.getState().definitionMap)).toEqual(['host-info', 'downloaded:clock']);

    useWidgetRegistryStore.getState().mergeDownloaded([{ ...downloaded, verified: false }]);
    expect(Object.keys(useWidgetRegistryStore.getState().definitionMap)).toEqual(['host-info']);
  });
});
