// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { clearPluginCapabilities, givePluginCapabilities } from '../../test/plugin-capabilities';
import {
  activeCapabilities,
  apiClientPath,
  fillPath,
  usePluginCapabilities,
  usePluginCapability,
} from './capabilities';

const vmProvider = { plugin: 'vm-plugin', kind: 'runbook-provider', domain: 'vm' };
const ctProvider = { plugin: 'ct-plugin', kind: 'runbook-provider', domain: 'ct' };
const subject = { plugin: 'vm-plugin', kind: 'lifecycle-subject', subjectType: 'vm', page: '/plugins/vm-plugin' };

describe('plugin capabilities', () => {
  afterEach(() => clearPluginCapabilities());

  it('keeps only capabilities of an enabled, loaded plugin', () => {
    const installed = [
      { slug: 'vm-plugin', version: '1', enabled: true, loaded: true },
      { slug: 'ct-plugin', version: '1', enabled: false, loaded: true },
    ];
    expect(activeCapabilities([vmProvider, ctProvider, subject], installed, 'runbook-provider')).toEqual([vmProvider]);
    expect(activeCapabilities([vmProvider], [], 'runbook-provider')).toEqual([]);
  });

  it('resolves a capability by its key and hides it while the plugin is not loaded', () => {
    givePluginCapabilities([vmProvider, ctProvider, subject]);
    const { result, rerender } = renderHook(() => usePluginCapability('runbook-provider', 'ct'));
    expect(result.current?.plugin).toBe('ct-plugin');

    givePluginCapabilities([vmProvider, ctProvider], ['ct-plugin']);
    rerender();
    expect(result.current).toBeUndefined();

    const all = renderHook(() => usePluginCapabilities('lifecycle-subject'));
    expect(all.result.current).toEqual([]);
  });

  it('turns a manifest route into an api-client path and escapes every placeholder', () => {
    expect(apiClientPath('/api/things')).toBe('/things');
    expect(fillPath('/api/things/backups/{id}', { id: 'a/b' })).toBe('/api/things/backups/a%2Fb');
    expect(fillPath('/plugins/x/{subject}/{missing}', { subject: 'v1' })).toBe('/plugins/x/v1/{missing}');
  });
});
