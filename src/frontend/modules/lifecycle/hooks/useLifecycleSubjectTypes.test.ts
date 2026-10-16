// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { clearPluginCapabilities, givePluginCapabilities } from '../../../test/plugin-capabilities';
import { useLifecycleSubjectTypes } from './useLifecycleSubjectTypes';

const HOST = ['', 'container', 'image', 'volume', 'network', 'stack'];

describe('lifecycle subject filter options', () => {
  afterEach(() => clearPluginCapabilities());

  it('offers only the host subjects when no plugin declares one', () => {
    givePluginCapabilities([]);
    const { result } = renderHook(() => useLifecycleSubjectTypes());
    expect(result.current.map((o) => o.value)).toEqual(HOST);
  });

  it('adds each declared plugin subject once, after the host subjects', () => {
    givePluginCapabilities([
      { plugin: 'guest-plugin', kind: 'lifecycle-subject', subjectType: 'vm', page: '/plugins/guest-plugin' },
      { plugin: 'widget-plugin', kind: 'lifecycle-subject', subjectType: 'widget', page: '/plugins/widget-plugin' },
      { plugin: 'widget-plugin-2', kind: 'lifecycle-subject', subjectType: 'widget', page: '/plugins/widget-plugin-2' },
      { plugin: 'guest-plugin', kind: 'estate-source', metric: 'vms', path: '/api/guests' },
    ]);
    const { result } = renderHook(() => useLifecycleSubjectTypes());
    expect(result.current.map((o) => o.value)).toEqual([...HOST, 'vm', 'widget']);
    expect(result.current.find((o) => o.value === 'vm')?.label).toBe('Virtual machine');
    expect(result.current.find((o) => o.value === 'widget')?.label).toBe('widget');
  });

  it('drops a plugin subject once its plugin is gone', () => {
    givePluginCapabilities(
      [{ plugin: 'guest-plugin', kind: 'lifecycle-subject', subjectType: 'vm', page: '/plugins/guest-plugin' }],
      ['guest-plugin'],
    );
    const { result } = renderHook(() => useLifecycleSubjectTypes());
    expect(result.current.map((o) => o.value)).toEqual(HOST);
  });
});
