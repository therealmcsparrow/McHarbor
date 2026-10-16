// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createSearchMatcher,
  createSearchPresetId,
  loadSearchPresets,
  matchesSearchFields,
  persistSearchPresets,
} from './search-filter';

describe('createSearchMatcher', () => {
  it('matches all values for blank queries', () => {
    const matcher = createSearchMatcher('   ', 'contains');

    expect(matcher.error).toBeNull();
    expect(matcher.matches('anything')).toBe(true);
  });

  it('matches contains queries as escaped case-insensitive text', () => {
    const matcher = createSearchMatcher('api.', 'contains');

    expect(matcher.error).toBeNull();
    expect(matcher.matches('API.gateway')).toBe(true);
    expect(matcher.matches('api-gateway')).toBe(false);
  });

  it('matches exact queries after trimming and lowercasing', () => {
    const matcher = createSearchMatcher(' Production ', 'exact');

    expect(matcher.matches('production')).toBe(true);
    expect(matcher.matches('production-api')).toBe(false);
  });

  it('matches valid regex queries and reports invalid regex patterns', () => {
    const matcher = createSearchMatcher('web-[0-9]+', 'regex');
    const invalid = createSearchMatcher('[', 'regex');

    expect(matcher.error).toBeNull();
    expect(matcher.matches('WEB-12')).toBe(true);
    expect(invalid.error).toBe('invalid');
    expect(invalid.matches('anything')).toBe(true);
  });
});

describe('matchesSearchFields', () => {
  it('matches across nullable field values', () => {
    expect(matchesSearchFields('db', 'contains', [undefined, null, 'primary-db'])).toEqual({
      error: null,
      matched: true,
    });
    expect(matchesSearchFields('cache', 'exact', ['cache-worker'])).toEqual({
      error: null,
      matched: false,
    });
  });
});

describe('search preset storage', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('loads persisted presets and ignores invalid values', () => {
    const presets = [{ id: 'one', label: 'One', state: { query: 'api' } }];

    persistSearchPresets('search:test', presets);

    expect(loadSearchPresets('search:test')).toEqual(presets);

    window.localStorage.setItem('search:test', '{"not":"an array"}');
    expect(loadSearchPresets('search:test')).toEqual([]);

    window.localStorage.setItem('search:test', '{');
    expect(loadSearchPresets('search:test')).toEqual([]);
  });

  it('ignores localStorage write failures', () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage disabled');
    });

    expect(() => persistSearchPresets('search:test', [])).not.toThrow();
    expect(setItem).toHaveBeenCalled();
  });
});

describe('createSearchPresetId', () => {
  it('uses randomUUID when available', () => {
    vi.stubGlobal('crypto', { randomUUID: () => 'preset-id' });

    expect(createSearchPresetId()).toBe('preset-id');
  });

  it('falls back to a timestamp ID', () => {
    vi.stubGlobal('crypto', {});
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-05-28T12:00:00Z'));

    expect(createSearchPresetId()).toBe('preset-1779969600000');
  });
});
