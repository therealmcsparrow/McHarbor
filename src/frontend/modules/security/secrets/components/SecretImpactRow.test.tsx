// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { SecretImpactResource } from '../types';
import { renderWithProviders } from '../../../../test/render';
import { SecretImpactRow } from './SecretImpactRow';

const resource = (over: Partial<SecretImpactResource>): SecretImpactResource => ({
  resourceType: 'module',
  resourceId: 'r1',
  resourceName: 'web-1',
  fields: [],
  appliedVersion: 1,
  stale: false,
  missing: false,
  supported: false,
  ...over,
});

function mount(r: SecretImpactResource) {
  renderWithProviders(
    <ul>
      <SecretImpactRow resource={r} version={2} selected={false} onSelectedChange={vi.fn()} disabled={false} />
    </ul>,
  );
}

describe('SecretImpactRow', () => {
  // A VM's cloud-init password is a one-time copy: telling the operator a
  // rotation "already reaches it" would be the opposite of true.
  it('labels a plugin reference as copied once, not as reached by rotation', () => {
    mount(resource({ resourceType: 'plugin', resourceId: 'addon-kvm:vm1', resourceName: 'mcawesome-dev' }));
    expect(screen.getByText('Plugin')).toBeInTheDocument();
    expect(screen.getByText('Copied once')).toBeInTheDocument();
    expect(screen.queryByText('Not applicable')).not.toBeInTheDocument();
  });

  it('keeps the generic label for other resources the vault cannot apply to', () => {
    mount(resource({ resourceType: 'module' }));
    expect(screen.getByText('Not applicable')).toBeInTheDocument();
    expect(screen.queryByText('Copied once')).not.toBeInTheDocument();
  });
});
