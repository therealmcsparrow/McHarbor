// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { useEnvironmentStore } from '@resources/stores/environment';
import { envParams, envUrl } from './env-url';

describe('envUrl', () => {
  beforeEach(() => {
    useEnvironmentStore.setState({ currentId: '', environments: [] });
  });

  it('returns the original path when there are no params or selected environment', () => {
    expect(envUrl('/containers')).toBe('/containers');
  });

  it('adds params and the selected environment', () => {
    useEnvironmentStore.getState().setCurrentId('env-1');

    expect(envUrl('/containers', { all: 'true' })).toBe('/containers?all=true&env=env-1');
  });

  it('overwrites an existing env param with the selected environment', () => {
    useEnvironmentStore.getState().setCurrentId('env-2');

    expect(envUrl('/containers', { env: 'old', all: 'false' })).toBe('/containers?env=env-2&all=false');
  });
});

describe('envParams', () => {
  beforeEach(() => {
    useEnvironmentStore.setState({ currentId: '', environments: [] });
  });

  it('returns extra params unchanged without a selected environment', () => {
    expect(envParams({ all: 'true' })).toEqual({ all: 'true' });
  });

  it('adds the selected environment to extra params', () => {
    useEnvironmentStore.getState().setCurrentId('env-3');

    expect(envParams({ all: 'true' })).toEqual({ all: 'true', env: 'env-3' });
  });
});
