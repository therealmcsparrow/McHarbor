// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { renderHook } from '@testing-library/react';
import i18n from 'i18next';
import { afterEach, beforeAll, describe, expect, it } from 'vitest';
import { clearPluginCapabilities, givePluginCapabilities } from '../../../test/plugin-capabilities';
import { useSourceLabel } from './useSourceLabel';

const declared = {
  plugin: 'guest-plugin',
  kind: 'backup-row-actions',
  sourceKind: 'guest',
  labelKey: 'backups.sourceLabel',
  labelNs: 'guests',
};

beforeAll(() => {
  i18n.addResourceBundle('en', 'guests', { backups: { sourceLabel: 'Guest machine' } }, true, true);
});

afterEach(() => clearPluginCapabilities());

describe('useSourceLabel', () => {
  it('titles a plugin source from its backup-row-actions declaration', () => {
    givePluginCapabilities([declared]);
    const { result } = renderHook(() => useSourceLabel());
    expect(result.current({ sourceKind: 'guest', sourceSlug: 'guest-plugin', sourceLabel: 'Guests' })).toBe(
      'Guest machine',
    );
    expect(result.current({ sourceKind: 'guest' })).toBe('Guest machine');
  });

  it('keeps the host label for its own container source', () => {
    givePluginCapabilities([declared]);
    const { result } = renderHook(() => useSourceLabel());
    expect(result.current({ sourceKind: 'container' })).toBe('Container');
  });

  it('never borrows another plugin declaration, and falls back to the provider name', () => {
    givePluginCapabilities([declared]);
    const { result } = renderHook(() => useSourceLabel());
    expect(result.current({ sourceKind: 'guest', sourceSlug: 'other-plugin', sourceLabel: 'Other' })).toBe('Other');
  });

  it('falls back to the provider name once the declaring plugin is gone', () => {
    givePluginCapabilities([declared], ['guest-plugin']);
    const { result } = renderHook(() => useSourceLabel());
    expect(result.current({ sourceKind: 'guest', sourceSlug: 'guest-plugin', sourceLabel: 'Guests' })).toBe('Guests');
  });
});
